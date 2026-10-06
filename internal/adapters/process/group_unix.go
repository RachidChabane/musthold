//go:build !windows

package process

import (
	"os/exec"
	"syscall"
)

// isolateGroup puts the command in its own process group and makes the
// cancellation kill the whole group, grandchildren included.
func isolateGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
