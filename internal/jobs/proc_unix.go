//go:build !windows

package jobs

import (
	"os/exec"
	"syscall"
)

// killGroup makes yt-dlp and its children (ffmpeg, deno) share a process group
// and kills the whole group on cancel, so no helper processes are left behind.
func killGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
