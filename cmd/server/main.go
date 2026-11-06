package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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

	maxDownloads, maxInfo := concurrencyLimits()

	jobManager, err := jobs.NewManager(jobs.Config{
		Dir:           envStr("DOWNLOAD_DIR", filepath.Join(os.TempDir(), "saveit")),
		MaxConcurrent: maxDownloads,
		JobTimeout:    time.Duration(envInt("DOWNLOAD_TIMEOUT_MIN", 45)) * time.Minute,
		TTL:           time.Duration(envInt("DOWNLOAD_TTL_MIN", 30)) * time.Minute,
		AbandonAfter:  2 * time.Minute,
	})
	if err != nil {
		log.Fatalf("download dir setup failed: %v", err)
	}

	ytdlp.StartAutoUpdater(12 * time.Hour)

	h := handlers.New(redisCache, jobManager, maxInfo)
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

// Each yt-dlp process (plus the Deno helper it runs for YouTube) peaks around
// 180-200 MB. On Kubernetes hosts one out-of-memory kill takes down the whole
// container, so size the limits from the container's memory unless set by env.
const perProcessMB = 200

func concurrencyLimits() (downloads, info int) {
	limitMB := memoryLimitMB()
	downloads, info = 2, 2
	if limitMB > 0 {
		budget := int((limitMB - 80) / perProcessMB)
		if budget < 2 {
			budget = 2
			log.Printf("WARNING: only %d MB of memory; YouTube lookups need ~%d MB each. Use at least 512 MB.", limitMB, perProcessMB)
		}
		if budget > 12 {
			budget = 12
		}
		info = budget / 3
		if info < 1 {
			info = 1
		}
		downloads = budget - info
	}
	downloads = envInt("MAX_CONCURRENT_DOWNLOADS", downloads)
	info = envInt("MAX_CONCURRENT_INFO", info)
	if limitMB > 0 {
		log.Printf("memory limit %d MB -> %d downloads + %d lookups at once", limitMB, downloads, info)
	} else {
		log.Printf("memory limit unknown -> %d downloads + %d lookups at once", downloads, info)
	}
	return downloads, info
}

// memoryLimitMB reads the container's cgroup memory limit (v2, then v1).
// Returns 0 when there is no limit or it can't be read.
func memoryLimitMB() int64 {
	for _, p := range []string{"/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/memory/memory.limit_in_bytes"} {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		v, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
		if err != nil || v <= 0 || v >= 1<<50 {
			return 0 // "max" or effectively unlimited
		}
		return v / (1 << 20)
	}
	return 0
}
