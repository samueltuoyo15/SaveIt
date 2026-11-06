// Package jobs runs yt-dlp downloads in the background, independent of any
// single HTTP request. Clients start a job, poll its progress, then fetch the
// finished file with a normal (resumable, Content-Length aware) GET.
//
// This replaces the old "hold the request open while yt-dlp downloads and
// merges the whole file" approach, which tripped proxy timeouts (Cloudflare
// drops requests that send no bytes for ~100s) and let unbounded parallel
// downloads get the container OOM-killed.
package jobs

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"Saveit/internal/ytdlp"
)

type Status string

const (
	StatusQueued  Status = "queued"
	StatusRunning Status = "running"
	StatusDone    Status = "done"
	StatusFailed  Status = "failed"
)

type Config struct {
	Dir           string        // where finished files live
	MaxConcurrent int           // simultaneous yt-dlp processes
	JobTimeout    time.Duration // hard cap for a single download
	TTL           time.Duration // how long finished files are kept for re-download
	AbandonAfter  time.Duration // cancel unfinished jobs nobody is polling
}

type Job struct {
	ID       string
	key      string
	url      string
	format   string
	title    string
	cancel   context.CancelFunc
	mu       sync.RWMutex
	status   Status
	phase    string
	part     int
	dl       int64
	total    int64
	speed    float64
	eta      int64
	path     string
	size     int64
	errMsg   string
	finished time.Time
	seen     time.Time
	users    int // clients waiting on this job; it is only cancelled when this hits 0
}

// Snapshot is the JSON shape returned to the client.
type Snapshot struct {
	ID       string  `json:"id"`
	Status   Status  `json:"status"`
	Phase    string  `json:"phase,omitempty"`
	Part     int     `json:"part,omitempty"`
	Progress float64 `json:"progress"`
	Done     int64   `json:"downloaded_bytes"`
	Total    int64   `json:"total_bytes"`
	Speed    float64 `json:"speed"`
	ETA      int64   `json:"eta"`
	Size     int64   `json:"size,omitempty"`
	Error    string  `json:"error,omitempty"`
}

func (j *Job) Snapshot() Snapshot {
	j.mu.RLock()
	defer j.mu.RUnlock()
	s := Snapshot{
		ID: j.ID, Status: j.status, Phase: j.phase, Part: j.part,
		Done: j.dl, Total: j.total, Speed: j.speed, ETA: j.eta,
		Size: j.size, Error: j.errMsg,
	}
	switch {
	case j.status == StatusDone:
		s.Progress = 100
	case j.total > 0:
		s.Progress = float64(j.dl) * 100 / float64(j.total)
		if s.Progress > 100 {
			s.Progress = 100
		}
	}
	return s
}

// File returns the finished file path and the download filename to offer.
func (j *Job) File() (path, filename string, ok bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.status != StatusDone {
		return "", "", false
	}
	j.seen = time.Now()
	return j.path, j.title + filepath.Ext(j.path), true
}

func (j *Job) touch() {
	j.mu.Lock()
	j.seen = time.Now()
	j.mu.Unlock()
}

type Manager struct {
	cfg  Config
	sem  chan struct{}
	mu   sync.Mutex
	byID map[string]*Job
	byK  map[string]*Job
}

func NewManager(cfg Config) (*Manager, error) {
	if cfg.MaxConcurrent < 1 {
		cfg.MaxConcurrent = 1
	}
	// Start from a clean slate: anything left over is from a previous process.
	_ = os.RemoveAll(cfg.Dir)
	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		return nil, err
	}
	m := &Manager{
		cfg:  cfg,
		sem:  make(chan struct{}, cfg.MaxConcurrent),
		byID: map[string]*Job{},
		byK:  map[string]*Job{},
	}
	go m.janitor()
	return m, nil
}

// Start returns an existing job for the same url+format (so repeat clicks and
// repeat visitors reuse an in-progress or finished download) or starts a new one.
func (m *Manager) Start(videoURL, format, title string) *Job {
	sum := sha256.Sum256([]byte(videoURL + "\x00" + format))
	key := hex.EncodeToString(sum[:])

	m.mu.Lock()
	defer m.mu.Unlock()
	if j, ok := m.byK[key]; ok {
		j.mu.Lock()
		reusable := j.status != StatusFailed
		if reusable {
			j.seen = time.Now()
			j.title = title
			j.users++
		}
		j.mu.Unlock()
		if reusable {
			return j
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), m.cfg.JobTimeout)
	j := &Job{
		ID: newID(), key: key, url: videoURL, format: format, title: title,
		cancel: cancel, status: StatusQueued, seen: time.Now(), users: 1,
	}
	m.byID[j.ID] = j
	m.byK[key] = j
	go m.run(ctx, j)
	return j
}

func (m *Manager) Get(id string) (*Job, bool) {
	m.mu.Lock()
	j, ok := m.byID[id]
	m.mu.Unlock()
	if ok {
		j.touch()
	}
	return j, ok
}

// Cancel withdraws one client's interest. The download only stops when no one
// else is waiting on the same job (same url+format shared between users).
func (m *Manager) Cancel(id string) bool {
	m.mu.Lock()
	j, ok := m.byID[id]
	m.mu.Unlock()
	if !ok {
		return false
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.status != StatusQueued && j.status != StatusRunning {
		return false
	}
	j.users--
	if j.users <= 0 {
		j.cancel()
		return true
	}
	return false
}

func (m *Manager) run(ctx context.Context, j *Job) {
	defer j.cancel()

	select {
	case m.sem <- struct{}{}:
		defer func() { <-m.sem }()
	case <-ctx.Done():
		if ctx.Err() == context.Canceled {
			m.fail(j, errors.New("download was cancelled"))
		} else {
			m.fail(j, errors.New("timed out waiting for a free download slot"))
		}
		return
	}

	j.mu.Lock()
	j.status = StatusRunning
	j.phase = "downloading"
	j.mu.Unlock()

	prefix := filepath.Join(m.cfg.Dir, j.ID)
	path, err := m.download(ctx, j, prefix)
	if err != nil {
		cleanup(prefix)
		m.fail(j, err)
		return
	}
	st, err := os.Stat(path)
	if err != nil {
		cleanup(prefix)
		m.fail(j, err)
		return
	}

	j.mu.Lock()
	j.status = StatusDone
	j.phase = ""
	j.path = path
	j.size = st.Size()
	j.finished = time.Now()
	j.mu.Unlock()
}

func (m *Manager) download(ctx context.Context, j *Job, prefix string) (string, error) {
	args := ytdlp.DownloadArgs(j.url, j.format, prefix+".%(ext)s")
	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	killGroup(cmd)
	cmd.WaitDelay = 5 * time.Second

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start yt-dlp: %w", err)
	}

	var lastErr string
	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			if m.handleLine(j, line) {
				continue
			}
			if strings.HasPrefix(line, "ERROR:") {
				lastErr = line
			}
		}
		_, _ = io.Copy(io.Discard, pr)
	}()

	waitErr := cmd.Wait()
	pw.Close()
	<-scanDone

	if waitErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", errors.New("download took too long and was stopped")
		}
		if ctx.Err() == context.Canceled {
			return "", errors.New("download was cancelled")
		}
		if lastErr != "" {
			return "", errors.New(ytdlp.CleanError(lastErr))
		}
		return "", fmt.Errorf("yt-dlp: %w", waitErr)
	}

	return findOutput(prefix)
}

// handleLine parses our custom progress template; returns true if consumed.
func (m *Manager) handleLine(j *Job, line string) bool {
	switch {
	case strings.HasPrefix(line, ytdlp.ProgressPrefix):
		f := strings.Fields(strings.TrimPrefix(line, ytdlp.ProgressPrefix))
		if len(f) < 5 {
			return true
		}
		dl := parseInt(f[0])
		total := parseInt(f[1])
		if total <= 0 {
			total = parseInt(f[2])
		}
		j.mu.Lock()
		if dl < j.dl/2 && j.dl > 0 {
			j.part++ // second stream (audio) of a merged download started
		}
		if j.part == 0 {
			j.part = 1
		}
		j.dl, j.total = dl, total
		j.speed, _ = strconv.ParseFloat(f[3], 64)
		j.eta = parseInt(f[4])
		j.phase = "downloading"
		j.mu.Unlock()
		return true
	case strings.HasPrefix(line, "[Merger]"), strings.HasPrefix(line, "[VideoRemuxer]"),
		strings.HasPrefix(line, "[FixupM3u8]"), strings.HasPrefix(line, "[FixupM4a]"):
		j.mu.Lock()
		j.phase = "merging"
		j.mu.Unlock()
		return true
	}
	return false
}

func (m *Manager) fail(j *Job, err error) {
	log.Printf("[jobs] %s failed: %v", j.ID, err)
	j.mu.Lock()
	j.status = StatusFailed
	j.phase = ""
	j.errMsg = err.Error()
	j.finished = time.Now()
	j.mu.Unlock()
}

func (m *Manager) janitor() {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for range t.C {
		now := time.Now()
		m.mu.Lock()
		for id, j := range m.byID {
			j.mu.RLock()
			status, seen, finished, path := j.status, j.seen, j.finished, j.path
			j.mu.RUnlock()

			switch status {
			case StatusQueued, StatusRunning:
				if now.Sub(seen) > m.cfg.AbandonAfter {
					j.cancel() // nobody is waiting for it any more
				}
				continue
			case StatusFailed:
				if now.Sub(finished) < time.Minute {
					continue
				}
			case StatusDone:
				if now.Sub(finished) < m.cfg.TTL || now.Sub(seen) < m.cfg.TTL {
					continue
				}
				// Open file handles keep reading fine after unlink on Linux.
				_ = os.Remove(path)
			}
			delete(m.byID, id)
			if m.byK[j.key] == j {
				delete(m.byK, j.key)
			}
		}
		m.mu.Unlock()
	}
}

func findOutput(prefix string) (string, error) {
	matches, _ := filepath.Glob(prefix + ".*")
	var best string
	var bestSize int64 = -1
	for _, p := range matches {
		base := filepath.Base(p)
		if strings.HasSuffix(base, ".part") || strings.HasSuffix(base, ".ytdl") ||
			strings.Contains(base, ".part-Frag") || strings.Contains(base, ".temp.") {
			continue
		}
		if st, err := os.Stat(p); err == nil && st.Size() > bestSize {
			best, bestSize = p, st.Size()
		}
	}
	if best == "" {
		return "", errors.New("download finished but no output file was produced")
	}
	return best, nil
}

func cleanup(prefix string) {
	matches, _ := filepath.Glob(prefix + ".*")
	for _, p := range matches {
		_ = os.Remove(p)
	}
}

func parseInt(s string) int64 {
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int64(f)
	}
	return 0
}

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
