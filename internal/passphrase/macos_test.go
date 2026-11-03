package passphrase

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMacDialogScriptNamesTheWindow(t *testing.T) {
	script := macDialogScript(`Enter passphrase for .env.enc: "now"`, 90)
	if !strings.Contains(script, `title "Pogo Password Request"`) {
		t.Fatalf("title missing: %s", script)
	}
	if !strings.Contains(script, `Pogo Password Request`) || !strings.Contains(script, `Enter passphrase for .env.enc:`) {
		t.Fatalf("body missing: %s", script)
	}
	if !strings.Contains(script, `\"now\"`) || strings.Contains(script, `for .env.enc: "now"`) {
		t.Fatalf("quote not escaped: %s", script)
	}
	if !strings.Contains(script, "giving up after 90") || !strings.Contains(script, `error "pogo timeout"`) {
		t.Fatalf("timeout missing: %s", script)
	}
}

func TestMacDialogAsksWhenDisplayIsUnset(t *testing.T) {
	dir := quietEnv(t)
	t.Cleanup(SetMacDialogForTest(false))
	oldOS, oldPath, oldOwner := goos, osascriptPath, consoleOwnerFn
	t.Cleanup(func() {
		goos, osascriptPath, consoleOwnerFn = oldOS, oldPath, oldOwner
	})
	goos = "darwin"
	consoleOwnerFn = func() (uint32, bool) { return uint32(os.Getuid()), true }
	log := filepath.Join(dir, "script")
	osascriptPath = writeExec(t, dir, "osascript", "#!/bin/sh\nprintf '%s' \"$2\" > "+log+"\nprintf '%s\\n' 'pogo-test-passphrase'\n")

	var got []byte
	var err error
	stderr := withStderr(t, func() {
		got, err = mustAcquire(t, Source{Prompt: "Enter passphrase: ", File: ".env.enc"})
	})
	if err != nil || string(got) != "pogo-test-passphrase" {
		t.Fatalf("%q %v", got, err)
	}
	if !strings.Contains(stderr, "Asking for the passphrase with macOS dialog") || !strings.Contains(stderr, `The window is titled "Pogo Password Request".`) {
		t.Fatalf("announce %q", stderr)
	}
	body, readErr := os.ReadFile(log)
	if readErr != nil || !strings.Contains(string(body), `title "Pogo Password Request"`) || !strings.Contains(string(body), "Enter passphrase for .env.enc:") {
		t.Fatalf("script %s %v", body, readErr)
	}
	d := Diagnose()
	if !d.Available || d.Backend != MacDialogLabel || !strings.Contains(d.Detail, MacDialogTitle) {
		t.Fatalf("%+v", d)
	}
}

func TestMacDialogCancelTimeoutAndSkip(t *testing.T) {
	dir := quietEnv(t)
	t.Cleanup(SetMacDialogForTest(false))
	oldOS, oldPath, oldOwner := goos, osascriptPath, consoleOwnerFn
	t.Cleanup(func() {
		goos, osascriptPath, consoleOwnerFn = oldOS, oldPath, oldOwner
	})
	goos = "darwin"
	consoleOwnerFn = func() (uint32, bool) { return uint32(os.Getuid()), true }

	osascriptPath = writeExec(t, dir, "osascript", "#!/bin/sh\necho '0:128: execution error: User canceled. (-128)' >&2\nexit 1\n")
	got, err := mustAcquire(t, Source{Prompt: "Enter passphrase: "})
	if !errors.Is(err, ErrCancelled) || got != nil {
		t.Fatalf("cancel %q %v", got, err)
	}

	osascriptPath = writeExec(t, dir, "osascript", "#!/bin/sh\necho '209:223: execution error: pogo timeout (2)' >&2\nexit 1\n")
	got, err = mustAcquire(t, Source{})
	var timed *PromptTimeoutError
	if !errors.As(err, &timed) || timed.What != MacDialogLabel || got != nil {
		t.Fatalf("timeout %q %v", got, err)
	}

	osascriptPath = writeExec(t, dir, "osascript", "#!/bin/sh\necho 'No user interaction allowed. (-1713)' >&2\nexit 1\n")
	got, err = mustAcquire(t, Source{})
	var np *NoPromptError
	if got != nil || !errors.As(err, &np) || !strings.Contains(err.Error(), "macOS dialog could not be shown") {
		t.Fatalf("not shown %q %v", got, err)
	}

	t.Setenv("SSH_CONNECTION", "203.0.113.5 40000 203.0.113.8 22")
	if ok, reason := macDialogAvailable(); ok || reason != "" {
		t.Fatalf("ssh %v %q", ok, reason)
	}
}

func TestAgentMarker(t *testing.T) {
	t.Setenv("OPENCODE", "")
	t.Setenv("AGENT", "")
	t.Setenv("POGO_FORCE_TERMINAL", "")
	if agentSession() {
		t.Fatal("empty env looked like an agent")
	}
	t.Setenv("OPENCODE", "1")
	if !agentSession() {
		t.Fatal("opencode")
	}
	t.Setenv("POGO_FORCE_TERMINAL", "1")
	if agentSession() {
		t.Fatal("force terminal")
	}
}
