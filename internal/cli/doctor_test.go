package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
