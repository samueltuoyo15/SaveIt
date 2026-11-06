package ytdlp

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36"

// ProgressPrefix marks our machine-readable progress lines on yt-dlp's output.
const ProgressPrefix = "SAVEIT_PROGRESS "

const progressTemplate = "download:" + ProgressPrefix +
	"%(progress.downloaded_bytes)s %(progress.total_bytes)s %(progress.total_bytes_estimate)s " +
	"%(progress.speed)s %(progress.eta)s"

// concurrentFragments is deliberately modest. The old value (up to 64) gave each
// download dozens of threads and buffers, and a couple of parallel downloads were
// enough to get yt-dlp OOM-killed ("signal: killed") on a small container.
func concurrentFragments() string {
	if v, err := strconv.Atoi(os.Getenv("YTDLP_CONCURRENT_FRAGMENTS")); err == nil && v > 0 && v <= 32 {
		return strconv.Itoa(v)
	}
	return "4"
}

func commonArgs() []string {
	return append(append([]string{}, jsRuntimeArgs()...),
		"--no-playlist",
		"--no-warnings",
		"--user-agent", userAgent,
		"--add-headers", "Accept-Language:en-US,en;q=0.9",
		"--no-check-certificates",
	)
}

func FetchInfo(ctx context.Context, videoURL string) ([]byte, error) {
	args := append([]string{"-j", "--skip-download"}, commonArgs()...)
	args = append(args, "--", videoURL)
	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, errors.New("timed out fetching video info")
		}
		return nil, errors.New(lastErrorLine(stderr.String()))
	}
	return out, nil
}

// DownloadArgs builds the yt-dlp arguments for a background download job.
func DownloadArgs(videoURL, formatID, outTemplate string) []string {
	args := []string{
		"-f", formatID,
		"--concurrent-fragments", concurrentFragments(),
		"--no-mtime",
		"--retries", "5",
		"--fragment-retries", "5",
		"--http-chunk-size", "10M",
		"--newline",
		"--progress",
		"--progress-delta", "0.5",
		"--progress-template", progressTemplate,
		"-o", outTemplate,
	}
	if strings.Contains(formatID, "+") {
		args = append(args, "--merge-output-format", "mp4")
	}
	args = append(args, commonArgs()...)
	return append(args, "--", videoURL)
}

var errPrefix = regexp.MustCompile(`^ERROR:\s*(\[[^\]]+\]\s*)?([A-Za-z0-9_-]+:\s*)?`)

// CleanError turns a raw yt-dlp "ERROR: [youtube] abc: Video unavailable" line
// into something presentable to users.
func CleanError(line string) string {
	msg := strings.TrimSpace(errPrefix.ReplaceAllString(line, ""))
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "sign in to confirm"), strings.Contains(lower, "not a bot"):
		return "The site is blocking automated downloads right now. Please try again in a few minutes."
	case strings.Contains(lower, "private"):
		return "This video is private."
	case strings.Contains(lower, "age-restricted"), strings.Contains(lower, "confirm your age"):
		return "This video is age-restricted and can't be downloaded."
	case strings.Contains(lower, "requested format is not available"):
		return "That quality is no longer available. Fetch the formats again and pick another."
	case strings.Contains(lower, "unavailable"), strings.Contains(lower, "removed"), strings.Contains(lower, "404"):
		return "This video is unavailable or has been removed."
	}
	if msg == "" {
		return "Could not process this video. Check the link and try again."
	}
	if len(msg) > 200 {
		msg = msg[:200] + "…"
	}
	return msg
}

func lastErrorLine(stderr string) string {
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(lines[i], "ERROR:") {
			log.Printf("[yt-dlp] %s", lines[i])
			return CleanError(lines[i])
		}
	}
	if len(lines) > 0 && lines[len(lines)-1] != "" {
		log.Printf("[yt-dlp] failed without ERROR line: %s", lines[len(lines)-1])
	}
	return CleanError("")
}
