//go:build linux

package cli

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

func TestOpenShortPassphraseTTY(t *testing.T) {
	if testing.Short() {
		t.Skip("full Argon2id")
	}
	dir := t.TempDir()
	plain := filepath.Join(dir, ".env")
	enc := filepath.Join(dir, "abcd.enc")
	body := "API_KEY=pogo_test_api_key_example\n"
	if err := os.WriteFile(plain, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("POGO_ABCD", "abcd")
	code, _, stderr := runCLI(t, "encrypt", plain, "-f", enc, "--key-env", "POGO_ABCD", "--allow-short-passphrase")
	if code != 0 {
		t.Fatalf("encrypt %d %s", code, stderr)
	}

	code, stdout, stderr := runCLIOnTTY(t, "abcd", "decrypt", "-f", enc)
	if code != 0 || stdout != body {
		t.Fatalf("tty decrypt %d %q %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "Enter passphrase: ") || strings.Contains(stderr, "min 15") || strings.Contains(stderr, "easier to guess") {
		t.Fatalf("open prompt %s", stderr)
	}

	code, stdout, stderr = runCLIOnTTY(t, "abcd", "run", "-f", enc, "--", "pogo-this-command-does-not-exist")
	if code != 1 || !strings.Contains(stderr, "Could not find command") || strings.Contains(stderr, "too short") || strings.Contains(stderr, "easier to guess") {
		t.Fatalf("tty run %d %q %s", code, stdout, stderr)
	}

	code, _, stderr = runCLIOnTTY(t, "", "decrypt", "-f", enc)
	if code != 4 || !strings.Contains(stderr, "Passphrase is empty") {
		t.Fatalf("tty empty %d %s", code, stderr)
	}

	code, _, stderr = runCLIOnTTY(t, "nope", "decrypt", "-f", enc)
	if code != 2 || !strings.Contains(stderr, "Decryption failed") {
		t.Fatalf("tty wrong %d %s", code, stderr)
	}
}

func runCLIOnTTY(t *testing.T, password string, args ...string) (int, string, string) {
	t.Helper()
	master, slave := openCLIPty(t)
	defer master.Close()
	defer slave.Close()

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer outR.Close()
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer errR.Close()

	oldIn, oldOut, oldErr := os.Stdin, os.Stdout, os.Stderr
	os.Stdin, os.Stdout, os.Stderr = slave, outW, errW
	defer func() {
		os.Stdin, os.Stdout, os.Stderr = oldIn, oldOut, oldErr
	}()

	done := make(chan int, 1)
	go func() {
		done <- Main(args, "1.0.0-test")
	}()

	prompt := waitCLIPrompt(t, errR, "Enter passphrase: ")
	if _, err := master.Write([]byte(password + "\n")); err != nil {
		t.Fatal(err)
	}

	var code int
	select {
	case code = <-done:
	case <-time.After(3 * time.Minute):
		t.Fatal("timed out waiting for pogo")
	}
	os.Stdin, os.Stdout, os.Stderr = oldIn, oldOut, oldErr
	_ = outW.Close()
	_ = errW.Close()
	stdout, err := io.ReadAll(outR)
	if err != nil {
		t.Fatal(err)
	}
	_ = errR.SetReadDeadline(time.Time{})
	rest, err := io.ReadAll(errR)
	if err != nil {
		t.Fatal(err)
	}
	return code, string(stdout), prompt + string(rest)
}

func openCLIPty(t *testing.T) (master, slave *os.File) {
	t.Helper()
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
	return master, slave
}

func waitCLIPrompt(t *testing.T, r *os.File, prompt string) string {
	t.Helper()
	_ = r.SetReadDeadline(time.Now().Add(5 * time.Second))
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
	return string(buf)
}
