package main

import (
	"os"

	"pogo/internal/cli"
	"pogo/internal/passphrase"
)

// version is the release printed by `pogo version`.
var version = "1.0.9"

func main() {
	if os.Getenv("POGO_HERDR_RESPONDER") == "1" {
		os.Exit(passphrase.HerdrResponder())
	}
	os.Exit(cli.Main(os.Args[1:], version))
}
