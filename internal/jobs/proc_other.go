//go:build windows

package jobs

import "os/exec"

func killGroup(cmd *exec.Cmd) {}
