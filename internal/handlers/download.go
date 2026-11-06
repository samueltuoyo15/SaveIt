package handlers

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"Saveit/utils"
)

var (
	formatIDRegex = regexp.MustCompile(`^[a-zA-Z0-9+_-]{1,64}$`)
	jobIDRegex    = regexp.MustCompile(`^[a-f0-9]{24}$`)
	unsafeName    = regexp.MustCompile(`[/\\?%*:|"<>\x00-\x1f]`)
)

// StartDownload: POST /api/download {url, format, title} -> job snapshot.
func (h *Handler) StartDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		URL    string `json:"url"`
		Format string `json:"format"`
		Title  string `json:"title"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.URL == "" || !utils.ValidateURL(req.URL) {
		writeError(w, http.StatusBadRequest, "missing or invalid url")
		return
	}
	if !formatIDRegex.MatchString(req.Format) {
		writeError(w, http.StatusBadRequest, "invalid format")
		return
	}
	job := h.Jobs.Start(req.URL, req.Format, sanitizeTitle(req.Title))
	writeJSON(w, http.StatusAccepted, job.Snapshot())
}

// CancelDownload: POST /api/download/cancel?id=... (also accepts sendBeacon).
func (h *Handler) CancelDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id := r.URL.Query().Get("id")
	if !jobIDRegex.MatchString(id) {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	stopped := h.Jobs.Cancel(id)
	writeJSON(w, http.StatusOK, map[string]bool{"stopped": stopped})
}

// DownloadStatus: GET /api/download/status?id=...
func (h *Handler) DownloadStatus(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if !jobIDRegex.MatchString(id) {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	job, ok := h.Jobs.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "This download has expired. Please start it again.")
		return
	}
	writeJSON(w, http.StatusOK, job.Snapshot())
}

// DownloadFile: GET /download/file?id=... serves the finished file with
// Content-Length and Range support, so browsers show real progress and can resume.
func (h *Handler) DownloadFile(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if !jobIDRegex.MatchString(id) {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	job, ok := h.Jobs.Get(id)
	if !ok {
		http.Error(w, "This download has expired. Go back and start it again.", http.StatusNotFound)
		return
	}
	path, name, ok := job.File()
	if !ok {
		http.Error(w, "This download isn't ready yet.", http.StatusConflict)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "This download has expired. Go back and start it again.", http.StatusGone)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.Error(w, "file error", http.StatusInternalServerError)
		return
	}

	ctype := mediaTypes[strings.ToLower(filepath.Ext(path))]
	if ctype == "" {
		ctype = mime.TypeByExtension(filepath.Ext(path))
	}
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Disposition", contentDisposition(name))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, "", st.ModTime(), f)
}

// Slim container images often lack /etc/mime.types, so map the usual outputs.
var mediaTypes = map[string]string{
	".mp4":  "video/mp4",
	".m4a":  "audio/mp4",
	".webm": "video/webm",
	".mkv":  "video/x-matroska",
	".mp3":  "audio/mpeg",
	".opus": "audio/ogg",
}

func sanitizeTitle(t string) string {
	t = strings.TrimSpace(unsafeName.ReplaceAllString(t, "-"))
	if r := []rune(t); len(r) > 120 {
		t = string(r[:120])
	}
	if t == "" {
		t = "video"
	}
	return t
}

// contentDisposition sets an ASCII fallback plus an RFC 5987 UTF-8 filename.
func contentDisposition(name string) string {
	ascii := strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, name)
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, ascii, url.PathEscape(name))
}
