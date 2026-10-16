// Package passphrase reads a passphrase and checks its length.
//
// Interactive entry uses a terminal with echo disabled. An environment
// variable is the other non-interactive source. Stdin is a pipe or a
// terminal: a pipe is read until EOF and trimmed, and a terminal prompts
// on stderr, turns echo off, and reads one line, the same as the interactive
// prompt. Pogo does not read a passphrase from a file. Confirmation (typing
// the passphrase twice) happens only for interactive entry.
package passphrase

import (
	"bytes"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"os"
	"unicode/utf8"

	"golang.org/x/term"
	"pogo/internal/securemem"
)

const (
	// DefaultMin is the recommended and default minimum length.
	DefaultMin = 15
	// AbsoluteMin is the lowest --min-length Pogo will accept.
	AbsoluteMin = 1
	// MaxBytes caps a passphrase so a huge stdin cannot be read into memory as one.
	MaxBytes = 4096
)

var (
	// ErrTooShort is mapped to exit code 4.
	ErrTooShort = errors.New("passphrase too short")
	// ErrTooLong means the passphrase exceeded MaxBytes.
	ErrTooLong = errors.New("passphrase too long")
	// ErrMismatch means the two interactive entries differed.
	ErrMismatch = errors.New("passphrases do not match")
)

// Source describes where the passphrase comes from.
// At most one of Env and Stdin may be set. If neither is set, Pogo prompts
// on the terminal. There is no file source.
//
// When Stdin is set and standard input is a terminal, Prompt is printed to
// stderr (the usual interactive prompt when Prompt is empty). A pipe ignores
// Prompt, reads until EOF, and trims the ends.
type Source struct {
	Env    string
	Stdin  bool
	Prompt string
}

// CheckLength reports ErrTooShort or ErrTooLong.
// Length is counted in Unicode characters, not bytes.
func CheckLength(pass []byte, min int) error {
	if len(pass) > MaxBytes {
		return ErrTooLong
	}
	if !utf8.Valid(pass) {
		return errors.New("passphrase is not valid UTF-8")
	}
	if utf8.RuneCount(pass) < min {
		return ErrTooShort
	}
	return nil
}

// Length returns the number of Unicode characters.
func Length(pass []byte) int {
	return utf8.RuneCount(pass)
}

// Acquire reads one passphrase.
func Acquire(src Source) ([]byte, error) {
	switch {
	case src.Env != "":
		return readEnv(src.Env)
	case src.Stdin:
		return readStdin(src.Prompt)
	default:
		prompt := src.Prompt
		if prompt == "" {
			prompt = "Enter passphrase (min 15 characters): "
		}
		return readInteractive(prompt)
	}
}

// AcquireConfirmed reads a passphrase. Interactive entry is typed twice.
// An environment variable or stdin is read once.
func AcquireConfirmed(src Source, firstPrompt, confirmPrompt string) ([]byte, error) {
	if src.Env != "" || src.Stdin {
		return Acquire(src)
	}
	if firstPrompt == "" {
		firstPrompt = "Enter passphrase (min 15 characters): "
	}
	if confirmPrompt == "" {
		confirmPrompt = "Enter it again to confirm: "
	}
	a, err := readInteractive(firstPrompt)
	if err != nil {
		return nil, err
	}
	b, err := readInteractive(confirmPrompt)
	if err != nil {
		securemem.Wipe(a)
		return nil, err
	}
	if !Match(a, b) {
		securemem.Wipe(a)
		securemem.Wipe(b)
		return nil, ErrMismatch
	}
	securemem.Wipe(b)
	return a, nil
}

// Match compares two passphrases without leaking the contents through == on strings.
func Match(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare(a, b) == 1
}

func readEnv(name string) ([]byte, error) {
	v, ok := os.LookupEnv(name)
	if !ok {
		return nil, fmt.Errorf("environment variable %s is not set", name)
	}
	// The process environment still holds the string; Go cannot wipe it.
	// Callers that start a child should drop this variable from the child.
	b := []byte(v)
	return b, nil
}

func readStdin(prompt string) ([]byte, error) {
	// A terminal is the interactive prompt: one line, echo off, no trim.
	// A pipe keeps the previous contract: read until EOF and trim the ends.
	// Confirmation is not asked here. Callers that want it use the
	// interactive path, which is the only place a passphrase is typed twice.
	if term.IsTerminal(int(os.Stdin.Fd())) {
		if prompt == "" {
			prompt = "Enter passphrase (min 15 characters): "
		}
		return readInteractive(prompt)
	}
	return readStdinPipe()
}

func readStdinPipe() ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(os.Stdin, MaxBytes+65))
	if err != nil {
		return nil, fmt.Errorf("cannot read passphrase from stdin: %w", err)
	}
	if len(b) > MaxBytes+64 {
		securemem.Wipe(b)
		return nil, ErrTooLong
	}
	trimmed := bytes.TrimSpace(b)
	out := make([]byte, len(trimmed))
	copy(out, trimmed)
	securemem.Wipe(b)
	return out, nil
}

func readInteractive(prompt string) ([]byte, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return nil, errors.New("no passphrase was provided, and stdin is not a terminal so Pogo cannot ask for one")
	}
	fmt.Fprint(os.Stderr, prompt)
	pw, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("could not read passphrase: %w", err)
	}
	return pw, nil
}
