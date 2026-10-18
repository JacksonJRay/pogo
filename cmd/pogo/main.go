package main

import (
	"os"

	"pogo/internal/cli"
)

// version is the release printed by `pogo version`.
var version = "1.0.5"

func main() {
	os.Exit(cli.Main(os.Args[1:], version))
}
