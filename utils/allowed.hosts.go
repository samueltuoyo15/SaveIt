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
	"threads.net",
	"reddit.com",
	"redd.it",
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
	"threads.net":   true,
	"reddit.com":    true,
	"redd.it":       true,
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
