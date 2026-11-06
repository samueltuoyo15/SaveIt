package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"Saveit/internal/formats"
	"Saveit/internal/ytdlp"
	"Saveit/utils"
)

type infoCall struct {
	done chan struct{}
	resp []byte
	err  error
}

func (h *Handler) VideoInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil || req.URL == "" {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !utils.ValidateURL(req.URL) {
		writeError(w, http.StatusBadRequest, "This site isn't supported, or the link is invalid.")
		return
	}

	// Only successful results are cached, so a failed fetch is retried next time.
	// Bump the version whenever the response shape or format selection changes,
	// so stale cached results are not served after a deploy.
	cacheKey := "info:v3:" + req.URL
	if cached, ok := h.Cache.Get(cacheKey); ok {
		w.Header().Set("X-Cache", "HIT")
		w.Header().Set("Content-Type", "application/json")
		w.Write(cached)
		return
	}

	resp, err := h.fetchInfoOnce(req.URL)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	h.Cache.Set(cacheKey, resp, 30*time.Minute)

	w.Header().Set("X-Cache", "MISS")
	w.Header().Set("Content-Type", "application/json")
	w.Write(resp)
}

// fetchInfoOnce collapses concurrent requests for the same URL into a single
// yt-dlp process. The fetch is detached from any one request, so a user
// refreshing doesn't kill the work another user is waiting on.
func (h *Handler) fetchInfoOnce(url string) ([]byte, error) {
	h.infoMu.Lock()
	if c, ok := h.infoCalls[url]; ok {
		h.infoMu.Unlock()
		<-c.done
		return c.resp, c.err
	}
	c := &infoCall{done: make(chan struct{})}
	h.infoCalls[url] = c
	h.infoMu.Unlock()

	defer func() {
		h.infoMu.Lock()
		delete(h.infoCalls, url)
		h.infoMu.Unlock()
		close(c.done)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	raw, err := ytdlp.FetchInfo(ctx, url)
	if err != nil {
		c.err = err
		return nil, err
	}
	info, err := formats.ParseInfo(raw)
	if err != nil {
		c.err = infoError("Could not read this video's details. Please try again.")
		return nil, c.err
	}
	if len(info.Formats) == 0 {
		c.err = errNoFormats
		return nil, c.err
	}
	c.resp, c.err = json.Marshal(info)
	return c.resp, c.err
}

type infoError string

func (e infoError) Error() string { return string(e) }

const errNoFormats = infoError("No downloadable formats were found for this link.")
