package ytdlp

import (
	"context"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// StartAutoUpdater runs `yt-dlp -U` now and then every interval. YouTube
// regularly changes things that make older yt-dlp versions fail with
// "HTTP Error 403: Forbidden", so a build-time install is not enough.
// Disable with YTDLP_AUTO_UPDATE=false.
func StartAutoUpdater(interval time.Duration) {
	if v := strings.ToLower(os.Getenv("YTDLP_AUTO_UPDATE")); v == "false" || v == "0" || v == "no" {
		log.Printf("[yt-dlp] auto-update disabled")
		return
	}
	go func() {
		for {
			selfUpdate()
			time.Sleep(interval)
		}
	}()
}

func selfUpdate() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "yt-dlp", "-U").CombinedOutput()
	msg := strings.TrimSpace(string(out))
	if i := strings.LastIndex(msg, "\n"); i >= 0 {
		msg = msg[i+1:]
	}
	if err != nil {
		log.Printf("[yt-dlp] update check failed: %v: %s", err, msg)
		return
	}
	log.Printf("[yt-dlp] %s", msg)
}

var (
	jsOnce sync.Once
	jsArgs []string
)

// jsRuntimeArgs: yt-dlp only enables deno by default. If deno is missing but
// node is installed (common on dev machines), tell yt-dlp to use node.
func jsRuntimeArgs() []string {
	jsOnce.Do(func() {
		if _, err := exec.LookPath("deno"); err == nil {
			return
		}
		if _, err := exec.LookPath("node"); err == nil {
			jsArgs = []string{"--js-runtimes", "node"}
		}
	})
	return jsArgs
}
