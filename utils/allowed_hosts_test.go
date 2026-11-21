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
