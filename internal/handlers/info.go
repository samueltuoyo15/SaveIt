package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"Saveit/internal/formats"
	"Saveit/internal/gallerydl"
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
	clientIP := utils.ClientIP(r)
	log.Printf("[info] request url=%s ip=%s", req.URL, clientIP)

	if !utils.ValidateURL(req.URL) {
		log.Printf("[info] unsupported or invalid url=%s ip=%s", req.URL, clientIP)
		writeError(w, http.StatusBadRequest, utils.UnsupportedMessage(req.URL))
		return
	}

	// Only successful results are cached, so a failed fetch is retried next time.
	// Bump the version whenever the response shape or format selection changes,
	// so stale cached results are not served after a deploy.
	cacheKey := "info:v6:" + req.URL
	if cached, ok := h.Cache.Get(cacheKey); ok {
		log.Printf("[info] cache hit url=%s ip=%s", req.URL, clientIP)
		w.Header().Set("X-Cache", "HIT")
		w.Header().Set("Content-Type", "application/json")
		w.Write(cached)
		return
	}

	url := req.URL
	start := time.Now()
	resp, err := h.fetchInfoOnce(url)
	if err != nil {
		// Not 502: proxies (Cloudflare, the host's gateway) treat 502 as "server down"
		// and replace our message with their own error page.
		log.Printf("[info] fetch failed url=%s: %v (took %v) ip=%s", url, err, time.Since(start).Round(time.Millisecond), clientIP)
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	log.Printf("[info] fetch success url=%s (took %v) ip=%s", url, time.Since(start).Round(time.Millisecond), clientIP)
	h.Cache.Set(cacheKey, resp, 30*time.Minute)

	w.Header().Set("X-Cache", "MISS")
	w.Header().Set("Content-Type", "application/json")
	w.Write(resp)
}

// fetchInfoOnce collapses concurrent requests for the same URL into a single
// worker process. The fetch is detached from any one request, so a user
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

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	select {
	case h.infoSlots <- struct{}{}:
		defer func() { <-h.infoSlots }()
	case <-ctx.Done():
		c.err = infoError("We're very busy right now. Please try again in a moment.")
		return nil, c.err
	}

	raw, err := ytdlp.FetchInfo(ctx, url)
	if err != nil {
		if isNoVideoError(err) {
			if gInfo, gErr := gallerydl.FetchPhotos(ctx, url); gErr == nil && gInfo != nil {
				if vInfo := formats.ParseGallery(gInfo); vInfo != nil && len(vInfo.Formats) > 0 {
					c.resp, c.err = json.Marshal(vInfo)
					return c.resp, c.err
				}
			}
		}
		c.err = err
		return nil, err
	}
	info, err := formats.ParseInfo(raw)
	if err != nil {
		c.err = infoError("Could not read this video's details. Please try again.")
		return nil, c.err
	}
	if len(info.Formats) == 0 {
		if gInfo, gErr := gallerydl.FetchPhotos(ctx, url); gErr == nil && gInfo != nil {
			if vInfo := formats.ParseGallery(gInfo); vInfo != nil && len(vInfo.Formats) > 0 {
				c.resp, c.err = json.Marshal(vInfo)
				return c.resp, c.err
			}
		}
		c.err = errNoFormats
		return nil, c.err
	}
	c.resp, c.err = json.Marshal(info)
	return c.resp, c.err
}

func isNoVideoError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no video") ||
		strings.Contains(msg, "photo post") ||
		strings.Contains(msg, "carousel") ||
		strings.Contains(msg, "not supported") ||
		strings.Contains(msg, "could not process this video")
}

type infoError string

func (e infoError) Error() string { return string(e) }

const errNoFormats = infoError("No downloadable formats were found for this link.")
