package main

import (
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"time"

	"Saveit/internal/cache"
	"Saveit/internal/handlers"

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

	h := handlers.New(redisCache)
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
	mux.HandleFunc("/download", h.Download)

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
