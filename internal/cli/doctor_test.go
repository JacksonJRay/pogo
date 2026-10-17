package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pogo/internal/passphrase"
)

func TestDoctorNoBackend(t *testing.T) {
	clearPromptEnv(t)
	t.Cleanup(passphrase.SetAskpassDirsForTest(nil))
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	old := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = old; r.Close() }()

	code, stdout, stderr := runCLI(t, "doctor")
	if code != 1 {
		t.Fatalf("code %d stderr %s stdout %s", code, stderr, stdout)
	}
	for _, part := range []string{
		"pogo: no way to ask for the passphrase here",
		"POGO_ASKPASS not set",
		"editor socket not set",
		"no display",
		"docker run -it",
		"--key-stdin",
		"Last resort",
		"--key-env",
		"If you are an AI agent:",
	} {
		if !strings.Contains(stdout, part) {
			t.Errorf("stdout missing %q\n%s", part, stdout)
		}
	}
	if strings.Contains(stdout+stderr, "passphrase=") {
		t.Fatalf("leaked %s %s", stdout, stderr)
	}
}

func TestAskpassTimeoutAndCancelMessages(t *testing.T) {
	clearPromptEnv(t)
	t.Cleanup(passphrase.SetAskpassDirsForTest(nil))
	t.Cleanup(passphrase.SetPromptTimeoutsForTest(200*time.Millisecond, 300*time.Millisecond))
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	script := filepath.Join(dir, "ask.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n/bin/sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("POGO_ASKPASS", script)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	old := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = old; r.Close() }()

	code, _, stderr := runCLI(t, "init", "-f", filepath.Join(dir, "new.enc"))
	if code != 1 || !strings.Contains(stderr, "pogo: the POGO_ASKPASS prompt got no answer within the timeout") || !strings.Contains(stderr, "Ways forward, in order:") || strings.Contains(stderr, "was cancelled") {
		t.Fatalf("timeout %d %s", code, stderr)
	}

	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = runCLI(t, "init", "-f", filepath.Join(dir, "other.enc"))
	if code != 1 || !strings.Contains(stderr, "The passphrase prompt was cancelled.") || strings.Contains(stderr, "got no answer") {
		t.Fatalf("cancel %d %s", code, stderr)
	}
}

func TestDoctorAskpassExitZero(t *testing.T) {
	clearPromptEnv(t)
	t.Cleanup(passphrase.SetAskpassDirsForTest(nil))
	dir := t.TempDir()
	script := filepath.Join(dir, "ask.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s\\n' 'secret-value'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("POGO_ASKPASS", script)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	old := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = old; r.Close() }()

	code, stdout, stderr := runCLI(t, "doctor")
	if code != 0 || !strings.Contains(stdout, "backend: POGO_ASKPASS") || strings.Contains(stdout, "secret-value") {
		t.Fatalf("code %d\n%s\n%s", code, stdout, stderr)
	}
	code, _, stderr = runCLI(t, "doctor", "extra")
	if code != 1 || !strings.Contains(stderr, "no arguments") {
		t.Fatalf("args %d %s", code, stderr)
	}
}
