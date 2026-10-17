//go:build unix

package passphrase

import (
	"os/exec"
	"syscall"
	"time"
)

// prepareCommand puts the helper in its own process group so a timeout
// kills the dialog and any child it spawned.
func prepareCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 200 * time.Millisecond
}
