//go:build unix

package cli

import "golang.org/x/sys/unix"

// bestEffortDisableCoreDump asks the OS not to write a core file. A core
// dump can contain passphrases and decrypted env text. Failure is ignored
// because some environments lock this limit.
func bestEffortDisableCoreDump() {
	var r unix.Rlimit
	_ = unix.Setrlimit(unix.RLIMIT_CORE, &r)
}
