package gallerydl

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseJSON(t *testing.T) {
	raw := []byte(`[
		[2, {"category": "twitter", "count": 2, "title": "Test Tweet"}],
		[3, "https://pbs.twimg.com/media/photo1.jpg", {"extension": "jpg", "filename": "photo1.jpg", "width": 1200, "height": 800, "filesize": 250000, "user": "testuser"}],
		[3, "https://pbs.twimg.com/media/photo2.jpg", {"extension": "jpg", "filename": "photo2.jpg", "width": 1200, "height": 800, "filesize": 350000}]
	]`)

	info, err := parseJSON(raw, "https://x.com/test/status/123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(info.Photos) != 2 {
		t.Fatalf("expected 2 photos, got %d", len(info.Photos))
	}

	if info.Author != "testuser" {
		t.Errorf("expected author testuser, got %s", info.Author)
	}

	if info.Title != "Test Tweet" {
		t.Errorf("expected title 'Test Tweet', got %s", info.Title)
	}

	if info.Photos[0].URL != "https://pbs.twimg.com/media/photo1.jpg" {
		t.Errorf("expected photo 1 url, got %s", info.Photos[0].URL)
	}
}

func TestCreateZip(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "zip_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	f1 := filepath.Join(tmpDir, "img1.jpg")
	f2 := filepath.Join(tmpDir, "img2.jpg")
	_ = os.WriteFile(f1, []byte("fake image 1"), 0644)
	_ = os.WriteFile(f2, []byte("fake image 2"), 0644)

	zipOut := filepath.Join(tmpDir, "photos.zip")
	if err := CreateZip(zipOut, []string{f1, f2}); err != nil {
		t.Fatalf("CreateZip failed: %v", err)
	}

	fi, err := os.Stat(zipOut)
	if err != nil || fi.Size() == 0 {
		t.Fatalf("zip file not created or empty: %v", err)
	}
}
