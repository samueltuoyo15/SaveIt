package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"Saveit./internal/formats"
	"Saveit./internal/ytdlp"
	"Saveit./utils"
)

func (h *Handler) VideoInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if !utils.ValidateURL(req.URL) {
		http.Error(w, "unsupported or invalid URL", http.StatusBadRequest)
		return
	}

	cacheKey := "info:" + req.URL
	if cached, ok := h.Cache.Get(cacheKey); ok {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		w.Write(cached)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	raw, err := ytdlp.FetchInfo(ctx, req.URL)
	if err != nil {
		http.Error(w, "failed to fetch video info, check the URL and try again", http.StatusBadGateway)
		return
	}

	info, err := formats.ParseInfo(raw)
	if err != nil {
		http.Error(w, "failed to parse video metadata", http.StatusInternalServerError)
		return
	}

	resp, _ := json.Marshal(info)
	h.Cache.Set(cacheKey, resp, 10*time.Minute)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.Write(resp)
}
