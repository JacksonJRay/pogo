//go:build linux

package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestConvertDeletePty(t *testing.T) {
	if testing.Short() {
		t.Skip("full Argon2id")
	}
	bin := builtPogo(t)
	body := []byte("API_KEY=pogo_test_api_key_example\n")
	pass := "pogo-test-passphrase"
	t.Setenv("POGO_TEST_PASS", pass)
	t.Setenv("POGO_TTY_TIMEOUT", "")

	t.Run("yes", func(t *testing.T) {
		plain, enc := ptyEnv(t, body)
		code, out := runConvertOnTerminal(t, bin, nil, "y\n", "convert", plain, "-f", enc, "--key-env", "POGO_TEST_PASS")
		if code != 0 || fileExistsOK(plain) || !fileExistsOK(enc) {
			t.Fatalf("code %d exists plain %v enc %v\n%s", code, fileExistsOK(plain), fileExistsOK(enc), out)
		}
		for _, phrase := range []string{"[y/N]", "Removed ", "not a secure wipe", "Checked that"} {
			if !strings.Contains(out, phrase) {
				t.Errorf("missing %q\n%s", phrase, out)
			}
		}
		if !strings.HasPrefix(readHead(t, enc), "POGO") {
			t.Fatal("bad header")
		}
	})
	t.Run("no", func(t *testing.T) {
		plain, enc := ptyEnv(t, body)
		code, out := runConvertOnTerminal(t, bin, nil, "n\n", "convert", plain, "-f", enc, "--key-env", "POGO_TEST_PASS")
		if code != 0 || !sameFile(t, plain, body) {
			t.Fatalf("code %d\n%s", code, out)
		}
		if strings.Contains(out, "Removed ") || !strings.Contains(out, "Left the plaintext") {
			t.Fatalf("output\n%s", out)
		}
	})
	t.Run("enter", func(t *testing.T) {
		plain, enc := ptyEnv(t, body)
		code, out := runConvertOnTerminal(t, bin, nil, "\n", "convert", plain, "-f", enc, "--key-env", "POGO_TEST_PASS")
		if code != 0 || !sameFile(t, plain, body) {
			t.Fatalf("code %d\n%s", code, out)
		}
		if strings.Contains(out, "Removed ") || !strings.Contains(out, "[y/N]") {
			t.Fatalf("output\n%s", out)
		}
	})
	t.Run("no tty", func(t *testing.T) {
		plain, enc := ptyEnv(t, body)
		code, out, errOut := runConvertNoTTY(t, bin, "convert", plain, "-f", enc, "--key-env", "POGO_TEST_PASS")
		if code != 0 || !sameFile(t, plain, body) {
			t.Fatalf("code %d\n%s\n%s", code, out, errOut)
		}
		if !strings.Contains(errOut, "No terminal") || !strings.Contains(errOut, "Left the plaintext") || strings.Contains(errOut, "Removed ") {
			t.Fatalf("stderr\n%s", errOut)
		}
	})
	t.Run("key stdin yes", func(t *testing.T) {
		plain, enc := ptyEnv(t, body)
		pr, pw, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer pr.Close()
		go func() {
			_, _ = pw.Write([]byte(pass))
			pw.Close()
		}()
		code, out := runConvertOnTerminal(t, bin, pr, "y\n", "convert", plain, "-f", enc, "--key-stdin")
		if code != 0 || fileExistsOK(plain) || !fileExistsOK(enc) {
			t.Fatalf("code %d\n%s", code, out)
		}
		if strings.Contains(out, pass) || !strings.Contains(out, "Removed ") {
			t.Fatalf("output\n%s", out)
		}
	})
}

func ptyEnv(t *testing.T, body []byte) (plain, enc string) {
	t.Helper()
	dir := t.TempDir()
	plain = filepath.Join(dir, ".env")
	enc = filepath.Join(dir, ".env.enc")
	if err := os.WriteFile(plain, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return plain, enc
}

func fileExistsOK(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func sameFile(t *testing.T, path string, body []byte) bool {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Log(err)
		return false
	}
	return bytes.Equal(got, body)
}

func readHead(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	buf := make([]byte, 4)
	n, err := f.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	return string(buf[:n])
}

func runConvertNoTTY(t *testing.T, bin string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(), "POGO_TTY_TIMEOUT=")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	err := cmd.Run()
	if err == nil {
		return 0, stdout.String(), stderr.String()
	}
	ee, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatal(err)
	}
	return ee.ExitCode(), stdout.String(), stderr.String()
}

func runConvertOnTerminal(t *testing.T, bin string, stdin *os.File, answer string, args ...string) (int, string) {
	t.Helper()
	master, slave := openCLIPty(t)
	defer master.Close()

	cmd := exec.Command(bin, args...)
	if stdin == nil {
		cmd.Stdin = slave
	} else {
		cmd.Stdin = stdin
	}
	cmd.Stdout = slave
	cmd.Stderr = slave
	ctty := 0
	if stdin != nil {
		ctty = 1
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: ctty}
	cmd.Env = append(os.Environ(), "POGO_TTY_TIMEOUT=")
	if err := cmd.Start(); err != nil {
		slave.Close()
		t.Fatal(err)
	}
	slave.Close()

	var mu sync.Mutex
	var buf bytes.Buffer
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		tmp := make([]byte, 256)
		for {
			n, err := master.Read(tmp)
			if n > 0 {
				mu.Lock()
				buf.Write(tmp[:n])
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	snapshot := func() string {
		mu.Lock()
		defer mu.Unlock()
		return buf.String()
	}

	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()

	deadline := time.Now().Add(3 * time.Minute)
	var answered bool
	var runErr error
	for {
		if !answered && strings.Contains(snapshot(), "[y/N] ") {
			if _, err := master.Write([]byte(answer)); err != nil {
				cmd.Process.Kill()
				t.Fatal(err)
			}
			answered = true
		}
		select {
		case runErr = <-waitErr:
			goto done
		case <-time.After(20 * time.Millisecond):
			if time.Now().After(deadline) {
				cmd.Process.Kill()
				t.Fatalf("timed out waiting for convert\n%s", snapshot())
			}
		}
	}
done:
	_ = master.Close()
	select {
	case <-readDone:
	case <-time.After(2 * time.Second):
	}
	out := snapshot()
	if !answered {
		t.Fatalf("exited before the delete prompt: %v\n%s", runErr, out)
	}
	if runErr == nil {
		return 0, out
	}
	ee, ok := runErr.(*exec.ExitError)
	if !ok {
		t.Fatal(runErr)
	}
	return ee.ExitCode(), out
}

var (
	pogoBinOnce sync.Once
	pogoBin     string
	pogoBinErr  error
)

func builtPogo(t *testing.T) string {
	t.Helper()
	pogoBinOnce.Do(func() {
		dir, err := os.MkdirTemp("", "pogo-delete-bin-")
		if err != nil {
			pogoBinErr = err
			return
		}
		pogoBin = filepath.Join(dir, "pogo")
		cmd := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags=-w", "-o", pogoBin, "./cmd/pogo")
		cmd.Dir = moduleRoot()
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		out, err := cmd.CombinedOutput()
		if err != nil {
			pogoBinErr = fmt.Errorf("%w: %s", err, out)
		}
	})
	if pogoBinErr != nil {
		t.Fatal(pogoBinErr)
	}
	return pogoBin
}

func moduleRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
