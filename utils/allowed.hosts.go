package utils

import (
	"net/url"
	"strings"
)

var allowedDomains = []string{
	"youtube.com",
	"youtu.be",
	"twitter.com",
	"x.com",
	"facebook.com",
	"fb.watch",
	"instagram.com",
	"tiktok.com",
	"linkedin.com",
	"snapchat.com",
	"pinterest.com",
	"vimeo.com",
	"twitch.tv",
	"discord.com",
	"bilibili.com",
	"rumble.com",
	"kick.com",
}

var AllowedHosts = map[string]bool{
	"youtube.com":   true,
	"youtu.be":      true,
	"twitter.com":   true,
	"x.com":         true,
	"facebook.com":  true,
	"fb.watch":      true,
	"instagram.com": true,
	"tiktok.com":    true,
	"linkedin.com":  true,
	"snapchat.com":  true,
	"pinterest.com": true,
	"vimeo.com":     true,
	"twitch.tv":     true,
	"discord.com":   true,
	"bilibili.com":  true,
	"rumble.com":    true,
	"kick.com":      true,
}

func ValidateURL(videoURL string) bool {
	parsed, err := url.ParseRequestURI(videoURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return false
	}
	for _, domain := range allowedDomains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

// PlatformName extracts a human-friendly platform or domain name from a URL.
// Returns empty string if the input is not a URL.
func PlatformName(videoURL string) string {
	parsed, err := url.ParseRequestURI(videoURL)
	if err != nil {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	for _, p := range []string{"www.", "m.", "web.", "v.", "mobile."} {
		host = strings.TrimPrefix(host, p)
	}
	if host == "" || !strings.Contains(host, ".") {
		return ""
	}

	known := map[string]string{
		"reddit.com":      "Reddit",
		"redd.it":         "Reddit",
		"threads.net":     "Threads",
		"vimeo.com":       "Vimeo",
		"snapchat.com":    "Snapchat",
		"rumble.com":      "Rumble",
		"kick.com":        "Kick",
		"bsky.app":        "Bluesky",
		"bilibili.com":    "Bilibili",
		"spotify.com":     "Spotify",
		"soundcloud.com":  "SoundCloud",
		"dailymotion.com": "Dailymotion",
	}
	if name, ok := known[host]; ok {
		return name
	}
	for domain, name := range known {
		if strings.HasSuffix(host, "."+domain) {
			return name
		}
	}

	parts := strings.Split(host, ".")
	if len(parts) >= 2 {
		base := parts[len(parts)-2]
		if len(base) > 0 {
			return strings.ToUpper(base[:1]) + base[1:]
		}
	}
	return host
}

// UnsupportedMessage returns a friendly error message customized with the platform name or URL status.
func UnsupportedMessage(videoURL string) string {
	platform := PlatformName(videoURL)
	if platform == "" {
		return "This is not a valid URL. Please paste a video link."
	}
	return "We don't support " + platform + " downloads right now. Try YouTube, TikTok, Instagram, Twitter, etc."
}
