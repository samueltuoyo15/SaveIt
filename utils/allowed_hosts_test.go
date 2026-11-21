package utils

import "testing"

func TestValidateURL(t *testing.T) {
	tests := []struct {
		url   string
		valid bool
	}{
		{"https://www.facebook.com/share/v/1DujXzigtr/", true},
		{"https://m.facebook.com/watch/?v=123", true},
		{"https://web.facebook.com/reel/123", true},
		{"https://fb.watch/xyz", true},
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", true},
		{"https://m.youtube.com/watch?v=dQw4w9WgXcQ", true},
		{"https://youtu.be/dQw4w9WgXcQ", true},
		{"https://www.linkedin.com/posts/activity-123", true},
		{"https://vm.tiktok.com/xyz", true},
		{"https://clips.twitch.tv/xyz", true},
		{"https://v.redd.it/xyz", false},
		{"https://www.threads.net/@zuck/post/123", false},
		{"https://evilfacebook.com/xyz", false},
		{"https://notfacebook.com", false},
		{"not-a-url", false},
		{"", false},
	}

	for _, tt := range tests {
		got := ValidateURL(tt.url)
		if got != tt.valid {
			t.Errorf("ValidateURL(%q) = %v; want %v", tt.url, got, tt.valid)
		}
	}
}

func TestUnsupportedMessage(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://www.reddit.com/r/videos/123", "We don't support Reddit downloads right now. Try YouTube, TikTok, Instagram, Twitter, etc."},
		{"https://v.redd.it/xyz", "We don't support Reddit downloads right now. Try YouTube, TikTok, Instagram, Twitter, etc."},
		{"https://www.threads.net/@zuck/post/123", "We don't support Threads downloads right now. Try YouTube, TikTok, Instagram, Twitter, etc."},
		{"https://vimeo.com/12345", "We don't support Vimeo downloads right now. Try YouTube, TikTok, Instagram, Twitter, etc."},
		{"not-a-url", "This is not a valid URL. Please paste a video link."},
		{"", "This is not a valid URL. Please paste a video link."},
		{"funny cats playing", "This is not a valid URL. Please paste a video link."},
	}

	for _, tt := range tests {
		got := UnsupportedMessage(tt.url)
		if got != tt.want {
			t.Errorf("UnsupportedMessage(%q) = %q; want %q", tt.url, got, tt.want)
		}
	}
}
