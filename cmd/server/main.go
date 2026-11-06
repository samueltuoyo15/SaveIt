package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"Saveit/internal/cache"
	"Saveit/internal/handlers"
	"Saveit/internal/jobs"
	"Saveit/internal/ytdlp"

	"github.com/joho/godotenv"
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	log.Printf("Saveit starting on %d CPU cores", runtime.NumCPU())

	if os.Getenv("RAILWAY_ENVIRONMENT") == "" && os.Getenv("DOCKER_ENV") == "" {
		_ = godotenv.Load()
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	redisCache, err := cache.New(redisURL)
	if err != nil {
		log.Fatalf("Redis connection failed: %v", err)
	}
	defer redisCache.Close()

	jobManager, err := jobs.NewManager(jobs.Config{
		Dir:           envStr("DOWNLOAD_DIR", filepath.Join(os.TempDir(), "saveit")),
		MaxConcurrent: envInt("MAX_CONCURRENT_DOWNLOADS", 2),
		JobTimeout:    time.Duration(envInt("DOWNLOAD_TIMEOUT_MIN", 45)) * time.Minute,
		TTL:           time.Duration(envInt("DOWNLOAD_TTL_MIN", 30)) * time.Minute,
		AbandonAfter:  2 * time.Minute,
	})
	if err != nil {
		log.Fatalf("download dir setup failed: %v", err)
	}

	ytdlp.StartAutoUpdater(12 * time.Hour)

	h := handlers.New(redisCache, jobManager)
	mux := http.NewServeMux()

	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.Handle("/public/", http.StripPrefix("/public/", http.FileServer(http.Dir("public"))))
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "public/sitemap.xml")
	})
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "public/robots.txt")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Only serve index.html if the path is exactly "/"
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "templates/index.html")
	})
	mux.HandleFunc("/api/info", h.VideoInfo)
	mux.HandleFunc("/api/download", h.StartDownload)
	mux.HandleFunc("/api/download/status", h.DownloadStatus)
	mux.HandleFunc("/api/download/cancel", h.CancelDownload)
	mux.HandleFunc("/download/file", h.DownloadFile)

	port := os.Getenv("PORT")
	if port == "" {
		port = "5000"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 30 * time.Second,
		WriteTimeout:      0,
		IdleTimeout:       120 * time.Second,
	}

	log.Printf("Saveit listening on :%s (GOMAXPROCS=%s)", port, strconv.Itoa(runtime.GOMAXPROCS(0)))
	log.Fatal(srv.ListenAndServe())
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return def
}
