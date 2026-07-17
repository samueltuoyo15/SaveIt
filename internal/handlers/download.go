package handlers

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"Saveit/internal/ytdlp"
	"Saveit/utils"
)

var formatIDRegex = regexp.MustCompile(`^[a-zA-Z0-9+_-]+$`)

func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	videoURL := q.Get("url")
	formatID := q.Get("format")
	filename := q.Get("filename")

	if videoURL == "" || !utils.ValidateURL(videoURL) {
		http.Error(w, "missing or invalid url", http.StatusBadRequest)
		return
	}
	if !formatIDRegex.MatchString(formatID) {
		http.Error(w, "invalid format id", http.StatusBadRequest)
		return
	}
	if filename == "" {
		filename = "video.mp4"
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Hour)
	defer cancel()

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")

	if _, err := ytdlp.StreamWithInfo(ctx, videoURL, formatID, w); err != nil {
		fmt.Printf("[download] error: %v\n", err)
	}
}
