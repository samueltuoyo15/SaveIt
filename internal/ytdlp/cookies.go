package ytdlp

import (
	"encoding/base64"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// YouTube blocks many datacenter IPs with "Sign in to confirm you're not a bot".
// Passing cookies from a logged-in (throwaway) account gets past it.
//
// Provide them with either:
//   YTDLP_COOKIES_B64  = base64 of a Netscape-format cookies.txt (easy for env var dashboards)
//   YTDLP_COOKIES_FILE = path to a cookies.txt (e.g. a mounted config file)
//
// yt-dlp rewrites the cookie file as YouTube refreshes cookies, and mounted
// config files are often read-only, so we always work on a private copy.

var (
	cookieOnce sync.Once
	cookiePath string
)

// InitCookies loads cookies at startup so problems show in the logs right away.
func InitCookies() { cookieOnce.Do(setupCookies) }

func cookieArgs() []string {
	cookieOnce.Do(setupCookies)
	if cookiePath == "" {
		return nil
	}
	return []string{"--cookies", cookiePath}
}

func setupCookies() {
	var data []byte
	if b64 := strings.TrimSpace(os.Getenv("YTDLP_COOKIES_B64")); b64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			log.Printf("[yt-dlp] YTDLP_COOKIES_B64 is not valid base64: %v", err)
			return
		}
		data = decoded
	} else if p := os.Getenv("YTDLP_COOKIES_FILE"); p != "" {
		b, err := os.ReadFile(p)
		if err != nil {
			log.Printf("[yt-dlp] cannot read YTDLP_COOKIES_FILE: %v", err)
			return
		}
		data = b
	} else {
		return
	}

	if !strings.Contains(string(data), "youtube.com") {
		log.Printf("[yt-dlp] cookies provided but none are for youtube.com; check the export")
	}
	dst := filepath.Join(os.TempDir(), "saveit-cookies.txt")
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		log.Printf("[yt-dlp] cannot write cookies copy: %v", err)
		return
	}
	cookiePath = dst
	log.Printf("[yt-dlp] using YouTube cookies (%d bytes)", len(data))
}
