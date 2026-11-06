package jobs

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestHandleLineParsesProgress(t *testing.T) {
	m := &Manager{}
	j := &Job{}
	if !m.handleLine(j, "SAVEIT_PROGRESS 261120 682263 NA 306799.1 1") {
		t.Fatal("progress line not consumed")
	}
	s := j.Snapshot()
	if s.Done != 261120 || s.Total != 682263 || s.ETA != 1 || s.Part != 1 {
		t.Fatalf("bad snapshot: %+v", s)
	}
	// total missing -> fall back to estimate; NA values must not panic
	m.handleLine(j, "SAVEIT_PROGRESS 300000 NA 900000 NA NA")
	if s := j.Snapshot(); s.Total != 900000 {
		t.Fatalf("estimate fallback failed: %+v", s)
	}
	// counter resets -> second stream of a merge
	m.handleLine(j, "SAVEIT_PROGRESS 1024 50000 NA 1 1")
	if s := j.Snapshot(); s.Part != 2 {
		t.Fatalf("expected part 2, got %+v", s)
	}
	m.handleLine(j, `[Merger] Merging formats into "x.mp4"`)
	if s := j.Snapshot(); s.Phase != "merging" {
		t.Fatalf("expected merging phase, got %+v", s)
	}
}

// End-to-end with the real yt-dlp binary against a local file.
func TestManagerDownloadsAndDedupes(t *testing.T) {
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		t.Skip("yt-dlp not installed")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	src := t.TempDir()
	clip := filepath.Join(src, "clip.mp4")
	gen := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc=size=320x240:rate=25",
		"-t", "3", "-c:v", "libx264", "-preset", "ultrafast", clip)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v %s", err, out)
	}
	srv := httptest.NewServer(http.FileServer(http.Dir(src)))
	defer srv.Close()

	m, err := NewManager(Config{
		Dir: filepath.Join(t.TempDir(), "dl"), MaxConcurrent: 1,
		JobTimeout: time.Minute, TTL: time.Minute, AbandonAfter: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	j := m.Start(srv.URL+"/clip.mp4", "best", "My Clip")
	if again := m.Start(srv.URL+"/clip.mp4", "best", "My Clip"); again != j {
		t.Fatal("same url+format should reuse the running job")
	}

	deadline := time.Now().Add(45 * time.Second)
	for {
		s := j.Snapshot()
		if s.Status == StatusFailed {
			t.Fatalf("job failed: %s", s.Error)
		}
		if s.Status == StatusDone {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out: %+v", s)
		}
		time.Sleep(100 * time.Millisecond)
	}

	path, name, ok := j.File()
	if !ok || name != "My Clip.mp4" {
		t.Fatalf("File() = %q %q %v", path, name, ok)
	}
	want, _ := os.Stat(clip)
	got, err := os.Stat(path)
	if err != nil || got.Size() != want.Size() {
		t.Fatalf("output size mismatch: %v %v", got, err)
	}
	if again := m.Start(srv.URL+"/clip.mp4", "best", "My Clip"); again != j {
		t.Fatal("finished job should be reused instead of re-downloading")
	}
}
