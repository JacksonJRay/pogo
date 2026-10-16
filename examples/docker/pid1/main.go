// pid1 is the process Pogo execs in the container entrypoint examples.
// It prints the variables it received, its pid, and can exit or wait for SIGTERM.
package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var keys = []string{
	"PORT",
	"HOST",
	"DATABASE_URL",
	"API_KEY",
	"JWT_SECRET",
	"QUOTED_VALUE",
	"SPACED_VALUE",
	"EQUALS_VALUE",
	"MULTILINE_VALUE",
	"VITE_PUBLIC_TITLE",
	"NEXT_PUBLIC_TITLE",
}

func main() {
	fmt.Printf("pid=%d\n", os.Getpid())
	comm, err := os.ReadFile("/proc/1/comm")
	if err != nil {
		fmt.Printf("comm=missing\n")
	} else {
		fmt.Printf("comm=%s\n", trimNL(comm))
	}
	for _, name := range []string{"POGO_PASSPHRASE", "POGO_KEY"} {
		if _, ok := os.LookupEnv(name); ok {
			fmt.Printf("%s_present=yes\n", name)
		} else {
			fmt.Printf("%s_present=no\n", name)
		}
	}
	for _, key := range keys {
		value, ok := os.LookupEnv(key)
		if !ok {
			fmt.Printf("POGO_ENV\t%s\tMISSING\n", key)
			continue
		}
		fmt.Printf("POGO_ENV\t%s\t%s\n", key, hex.EncodeToString([]byte(value)))
	}
	mode := "print"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	switch mode {
	case "exit42":
		os.Exit(42)
	case "waitterm":
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
		select {
		case <-ch:
			fmt.Println("got-signal")
			os.Exit(0)
		case <-time.After(45 * time.Second):
			fmt.Println("timeout")
			os.Exit(9)
		}
	default:
		os.Exit(0)
	}
}

func trimNL(b []byte) string {
	if len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	return string(b)
}
