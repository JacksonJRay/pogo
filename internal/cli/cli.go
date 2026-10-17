// Package cli is the pogo command-line interface.
package cli

import (
	"fmt"
	"os"
)

// Main runs pogo and returns the process exit code.
// version is printed by `pogo version`.
func Main(args []string, version string) int {
	bestEffortDisableCoreDump()
	if len(args) == 0 {
		fmt.Fprint(os.Stdout, globalHelp())
		fmt.Fprintln(os.Stderr, "No command given. The quick start is above. Run `pogo help` for the same page.")
		return 1
	}
	p, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	if p.showHelp {
		fmt.Fprint(os.Stdout, p.helpText)
		return 0
	}
	if p.showVer || p.cmd == "version" {
		fmt.Fprintf(os.Stdout, "pogo %s\n", version)
		return 0
	}
	code, err := dispatch(p.cmd, p.opts, version)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
	}
	return code
}
