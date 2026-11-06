package handlers

import (
	"encoding/json"
	"net/http"
	"sync"

	"Saveit/internal/cache"
	"Saveit/internal/jobs"
)

type Handler struct {
	Cache *cache.Cache
	Jobs  *jobs.Manager

	infoMu    sync.Mutex
	infoCalls map[string]*infoCall
	infoSlots chan struct{} // caps simultaneous yt-dlp info lookups (memory)
}

func New(c *cache.Cache, j *jobs.Manager, maxInfo int) *Handler {
	if maxInfo < 1 {
		maxInfo = 1
	}
	return &Handler{Cache: c, Jobs: j, infoCalls: map[string]*infoCall{}, infoSlots: make(chan struct{}, maxInfo)}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError always returns JSON so the frontend never has to render raw text/HTML.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
