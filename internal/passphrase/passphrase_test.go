package passphrase

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCheckLength(t *testing.T) {
	if err := CheckLength([]byte("short-pass"), DefaultMin); err != ErrTooShort {
		t.Fatalf("%v", err)
	}
	if err := CheckLength([]byte("pogo-test-passphrase"), DefaultMin); err != nil {
		t.Fatal(err)
	}
	if err := CheckLength([]byte("pogo-test-passphrase"), 21); err != ErrTooShort {
		t.Fatalf("custom min: %v", err)
	}
	// 15 runes, more than 15 bytes.
	if err := CheckLength([]byte("äääääääääääääää"), DefaultMin); err != nil {
		t.Fatal(err)
	}
	long := make([]byte, MaxBytes+1)
	for i := range long {
		long[i] = 'a'
	}
	if err := CheckLength(long, 1); err != ErrTooLong {
		t.Fatalf("%v", err)
	}
}

func TestIsBlank(t *testing.T) {
	for _, pass := range []string{"", " ", "   ", "\t\r\n", " \u00a0 "} {
		if !IsBlank([]byte(pass)) {
			t.Fatalf("expected blank %q", pass)
		}
	}
	for _, pass := range []string{"a", " short ", "pogo-test-passphrase"} {
		if IsBlank([]byte(pass)) {
			t.Fatalf("expected non-blank %q", pass)
		}
	}
}

func TestMatch(t *testing.T) {
	if !Match([]byte("abc"), []byte("abc")) {
		t.Fatal("equal")
	}
	if Match([]byte("abc"), []byte("abd")) || Match([]byte("abc"), []byte("abcd")) {
		t.Fatal("unequal")
	}
}

func TestReadEnvExact(t *testing.T) {
	t.Setenv("POGO_TEST_PASS", "pogo-test-passphrase")
	got, err := Acquire(Source{Env: "POGO_TEST_PASS"})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "pogo-test-passphrase" {
		t.Fatalf("%q", got)
	}
	if _, err := Acquire(Source{Env: "POGO_TEST_PASS_MISSING"}); err == nil {
		t.Fatal("expected missing env error")
	}
}

func TestReadStdinPipeTrimsAndReadsUntilEOF(t *testing.T) {
	got, stderr, err := acquireStdinPipe(t, "  pogo-test-passphrase \r\n")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "pogo-test-passphrase" {
		t.Fatalf("trimmed passphrase %q", got)
	}
	if strings.Contains(stderr, "Enter passphrase") {
		t.Fatalf("pipe printed a prompt: %q", stderr)
	}

	// A pipe is not one line. The reader blocks until EOF, then trims only the ends.
	got, _, err = acquireStdinPipe(t, "  pogo-test-passphrase\nextra  ")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "pogo-test-passphrase\nextra" {
		t.Fatalf("pipe passphrase %q", got)
	}
}

func TestReadStdinPipeWaitsForEOF(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	old := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = old }()

	type result struct {
		pass []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		pass, err := Acquire(Source{Stdin: true, Prompt: "Enter passphrase (min 15 characters): "})
		done <- result{pass, err}
	}()
	if _, err := w.Write([]byte("pogo-test-passphrase\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case res := <-done:
		t.Fatalf("returned before EOF: %q %v", res.pass, res.err)
	case <-time.After(200 * time.Millisecond):
	}
	if _, err := w.Write([]byte("extra")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case res := <-done:
		if res.err != nil {
			t.Fatal(res.err)
		}
		if string(res.pass) != "pogo-test-passphrase\nextra" {
			t.Fatalf("%q", res.pass)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out after EOF")
	}
}

func TestReadStdinPipeTooLong(t *testing.T) {
	payload := bytes.Repeat([]byte("a"), MaxBytes+65)
	_, _, err := acquireStdinPipe(t, string(payload))
	if !errors.Is(err, ErrTooLong) {
		t.Fatalf("%v", err)
	}
}

func acquireStdinPipe(t *testing.T, input string) ([]byte, string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer errR.Close()
	oldIn, oldErr := os.Stdin, os.Stderr
	os.Stdin, os.Stderr = r, errW
	defer func() { os.Stdin, os.Stderr = oldIn, oldErr }()

	if _, err := w.Write([]byte(input)); err != nil {
		w.Close()
		t.Fatal(err)
	}
	w.Close()
	pass, acquireErr := Acquire(Source{Stdin: true, Prompt: "Enter passphrase (min 15 characters): "})
	errW.Close()
	stderr, _ := io.ReadAll(errR)
	return pass, string(stderr), acquireErr
}
