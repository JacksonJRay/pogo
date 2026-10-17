// Package passphrase reads a passphrase and checks its length.
//
// Interactive entry uses a terminal with echo disabled. When stdin is not a
// terminal and no key flag is set, Pogo asks through a prompt already on the
// machine (POGO_ASKPASS, the editor Git prompt, then a desktop askpass).
// An environment variable is the other non-interactive source. Stdin is a
// pipe or a terminal: a pipe is read until EOF and trimmed, and a terminal
// prompts on stderr, turns echo off, and reads one line. Pogo does not read
// a passphrase from a file. Confirmation (typing the passphrase twice)
// happens for the terminal and for those out-of-band prompts. --key-env and
// --key-stdin stay one-shot.
package passphrase

import (
	"bytes"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"
	"unicode/utf8"

	"golang.org/x/term"
	"pogo/internal/securemem"
)

const (
	// DefaultMin is the minimum length when creating a passphrase.
	DefaultMin = 15
	// MaxBytes caps a passphrase so a huge stdin cannot be read into memory as one.
	MaxBytes = 4096
)

var (
	// ErrTooShort is mapped to exit code 4.
	ErrTooShort = errors.New("passphrase too short")
	// ErrBlank means the passphrase is empty or only whitespace.
	// Callers map it to exit code 4. Skipping the 15-character minimum
	// does not allow a blank passphrase.
	ErrBlank = errors.New("passphrase is empty or only whitespace")
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
//
// File is the encrypted file name included in an out-of-band prompt. The
// terminal prompt is the Prompt string itself.
//
// Kind selects the editor box title. Cursor shows that host and not the
// prompt text. KindCurrent, KindNew, and KindConfirm are distinct hosts.
type Source struct {
	Env    string
	Stdin  bool
	Prompt string
	File   string
	Kind   PromptKind
}

// PromptKind is which passphrase is being asked for.
// Cursor titles its box from the host, so each kind has its own .invalid name.
type PromptKind int

const (
	// KindCurrent opens an existing file, including rekey's current passphrase.
	KindCurrent PromptKind = iota
	// KindNew is the first entry when a passphrase is created.
	KindNew
	// KindConfirm is the second entry when a passphrase is created.
	KindConfirm
)

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

// IsBlank reports whether pass is empty or only Unicode whitespace.
// A passphrase is still required when the 15-character minimum is skipped.
func IsBlank(pass []byte) bool {
	return len(bytes.TrimSpace(pass)) == 0
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
		return readFromUser(prompt, src.File, src.Kind)
	}
}

// AcquireConfirmed reads a passphrase. Interactive entry is typed twice,
// through whichever backend is selected, with KindNew and then KindConfirm.
// An environment variable or stdin is read once and is not confirmed.
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
	a, err := readFromUser(firstPrompt, src.File, KindNew)
	if err != nil {
		securemem.Wipe(a)
		return nil, err
	}
	b, err := readFromUser(confirmPrompt, src.File, KindConfirm)
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

// readFromUser prompts on a terminal, or through an out-of-band backend
// when stdin is not a terminal.
func readFromUser(prompt, file string, kind PromptKind) ([]byte, error) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		return readInteractive(prompt)
	}
	return promptOutOfBand(prompt, file, kind)
}

// ttyTimeout is opt-in. Unset or invalid means a real terminal waits until
// the person types or presses Ctrl-C, which is the normal interactive case.
// POGO_TTY_TIMEOUT is a positive number of seconds for a harness that
// allocated a terminal nobody is watching.
func ttyTimeout() time.Duration {
	v := os.Getenv("POGO_TTY_TIMEOUT")
	if v == "" {
		return 0
	}
	sec, err := strconv.Atoi(v)
	if err != nil || sec <= 0 {
		debug("POGO_TTY_TIMEOUT is not a positive number of seconds; the terminal will wait")
		return 0
	}
	return time.Duration(sec) * time.Second
}

func readInteractive(prompt string) ([]byte, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return nil, errors.New("no passphrase was provided, and stdin is not a terminal so Pogo cannot ask for one")
	}
	fmt.Fprint(os.Stderr, prompt)
	timeout := ttyTimeout()
	if timeout <= 0 {
		debug("terminal prompt waits until input or Ctrl-C")
		pw, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return nil, fmt.Errorf("could not read passphrase: %w", err)
		}
		return pw, nil
	}
	debug("terminal prompt timeout is " + timeout.String())
	type result struct {
		pw  []byte
		err error
	}
	ch := make(chan result, 1)
	go func() {
		pw, err := term.ReadPassword(fd)
		ch <- result{pw, err}
	}()
	select {
	case res := <-ch:
		fmt.Fprintln(os.Stderr)
		if res.err != nil {
			return nil, fmt.Errorf("could not read passphrase: %w", res.err)
		}
		return res.pw, nil
	case <-time.After(timeout):
		fmt.Fprintln(os.Stderr)
		return nil, fmt.Errorf("timed out waiting for a passphrase on the terminal after %s.\nA terminal waits until you type or press Ctrl-C unless POGO_TTY_TIMEOUT is set to a number of seconds.", timeout)
	}
}
