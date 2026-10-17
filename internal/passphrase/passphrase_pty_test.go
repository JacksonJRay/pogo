//go:build linux

package passphrase

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestReadStdinTerminalOneLineNoEcho(t *testing.T) {
	master, slave := openPTY(t)
	defer master.Close()
	defer slave.Close()

	errR, errW := stderrPipe(t)
	oldIn, oldErr := os.Stdin, os.Stderr
	os.Stdin, os.Stderr = slave, errW
	defer func() {
		os.Stdin, os.Stderr = oldIn, oldErr
		errW.Close()
	}()

	const prompt = "Enter the current passphrase: "
	const line = "pogo-test-passphrase "
	type result struct {
		pass []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		pass, err := Acquire(Source{Stdin: true, Prompt: prompt})
		done <- result{pass, err}
	}()

	waitForPrompt(t, errR, prompt)
	waitEchoOff(t, slave)
	if _, err := master.Write([]byte(line + "\nleftover-line\n")); err != nil {
		t.Fatal(err)
	}

	var res result
	select {
	case res = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("still waiting after one line; --key-stdin on a terminal must not wait for EOF")
	}
	if res.err != nil {
		t.Fatal(res.err)
	}
	// The terminal path matches the interactive prompt: one line, newline
	// removed, surrounding spaces kept. The pipe path trims those spaces.
	if string(res.pass) != line {
		t.Fatalf("passphrase %q", res.pass)
	}
	assertNoEcho(t, master, line)

	_ = slave.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 64)
	n, err := slave.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "leftover-line\n" {
		t.Fatalf("bytes after the first line: %q", buf[:n])
	}
}

func TestReadStdinTerminalConfirmedOnce(t *testing.T) {
	master, slave := openPTY(t)
	defer master.Close()
	defer slave.Close()

	errR, errW := stderrPipe(t)
	oldIn, oldErr := os.Stdin, os.Stderr
	os.Stdin, os.Stderr = slave, errW
	defer func() {
		os.Stdin, os.Stderr = oldIn, oldErr
		errW.Close()
	}()

	const prompt = "Enter passphrase (min 15 characters): "
	done := make(chan error, 1)
	var got []byte
	go func() {
		var err error
		got, err = AcquireConfirmed(Source{Stdin: true, Prompt: prompt}, prompt, "Enter it again to confirm: ")
		done <- err
	}()

	waitForPrompt(t, errR, prompt)
	waitEchoOff(t, slave)
	if _, err := master.Write([]byte("pogo-test-passphrase\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("confirmation was requested, or the read did not finish after one line")
	}
	if string(got) != "pogo-test-passphrase" {
		t.Fatalf("%q", got)
	}
	errW.Close()
	_ = errR.SetReadDeadline(time.Time{})
	stderr, err := io.ReadAll(errR)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stderr), "again to confirm") {
		t.Fatalf("terminal --key-stdin asked twice: %q", stderr)
	}
}

func openPTY(t *testing.T) (master, slave *os.File) {
	t.Helper()
	// A real terminal waits. An ambient POGO_TTY_TIMEOUT must not change that.
	t.Setenv("POGO_TTY_TIMEOUT", "")
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("ptmx: %v", err)
	}
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		master.Close()
		t.Fatal(err)
	}
	n, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		master.Close()
		t.Fatal(err)
	}
	slave, err = os.OpenFile("/dev/pts/"+strconv.Itoa(n), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		t.Fatal(err)
	}
	if !isTerminalFD(slave) {
		slave.Close()
		master.Close()
		t.Fatal("pty slave is not a terminal")
	}
	return master, slave
}

func isTerminalFD(f *os.File) bool {
	_, err := unix.IoctlGetTermios(int(f.Fd()), unix.TCGETS)
	return err == nil
}

func stderrPipe(t *testing.T) (r, w *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	return r, w
}

func waitForPrompt(t *testing.T, r *os.File, prompt string) {
	t.Helper()
	_ = r.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 0, 128)
	tmp := make([]byte, 128)
	for !strings.Contains(string(buf), prompt) {
		n, err := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			t.Fatalf("prompt %q: %v", buf, err)
		}
	}
}

func waitEchoOff(t *testing.T, slave *os.File) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		termios, err := unix.IoctlGetTermios(int(slave.Fd()), unix.TCGETS)
		if err == nil && termios.Lflag&unix.ECHO == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("echo stayed on")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestTTYIgnoresAskpassAndEditor(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ask.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho leaked\nexit 99\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("POGO_ASKPASS", script)
	t.Setenv("VSCODE_GIT_IPC_HANDLE", filepath.Join(dir, "missing.sock"))
	t.Setenv("POGO_TTY_TIMEOUT", "")

	master, slave := openPTY(t)
	defer master.Close()
	defer slave.Close()
	old := os.Stdin
	os.Stdin = slave
	defer func() { os.Stdin = old }()

	done := make(chan resultTTY, 1)
	go func() {
		pass, err := Acquire(Source{Prompt: "Enter passphrase: "})
		done <- resultTTY{pass, err}
	}()
	time.Sleep(50 * time.Millisecond)
	if _, err := master.Write([]byte("pogo-test-passphrase\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case res := <-done:
		if res.err != nil {
			t.Fatal(res.err)
		}
		if string(res.pass) != "pogo-test-passphrase" {
			t.Fatalf("%q", res.pass)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("terminal prompt did not read the typed passphrase")
	}
}

func TestTTYTimeoutOptIn(t *testing.T) {
	master, slave := openPTY(t)
	defer master.Close()
	defer slave.Close()
	t.Setenv("POGO_TTY_TIMEOUT", "1")
	old := os.Stdin
	os.Stdin = slave
	defer func() { os.Stdin = old }()

	start := time.Now()
	_, err := Acquire(Source{Prompt: "Enter passphrase: "})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("%v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("terminal timeout hung")
	}
	// Closing the slave unblocks the read that is still sitting in the timeout goroutine.
	slave.Close()
}

type resultTTY struct {
	pass []byte
	err  error
}

func assertNoEcho(t *testing.T, master *os.File, secret string) {
	t.Helper()
	// A deadline on /dev/ptmx does not interrupt Read, so poll instead.
	fds := []unix.PollFd{{Fd: int32(master.Fd()), Events: unix.POLLIN}}
	n, err := unix.Poll(fds, 100)
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		return
	}
	buf := make([]byte, 256)
	nread, err := unix.Read(int(master.Fd()), buf)
	if nread > 0 && strings.Contains(string(buf[:nread]), strings.TrimSpace(secret)) {
		t.Fatalf("passphrase was echoed: %q", buf[:nread])
	}
	if err != nil && err != unix.EAGAIN && err != unix.EWOULDBLOCK {
		t.Fatal(err)
	}
}
