//go:build unix

package cli

import "syscall"

func execCommand(path string, argv []string, env []string) error {
	return syscall.Exec(path, argv, env)
}
