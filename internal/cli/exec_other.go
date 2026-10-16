//go:build !unix

package cli

import "errors"

func execCommand(path string, argv []string, env []string) error {
	return errors.New("pogo run is supported on Linux and macOS")
}
