//go:build !unix

package passphrase

import "os/exec"

func prepareCommand(cmd *exec.Cmd) {}
