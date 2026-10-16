package editfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditRewritesAndRemoves(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ed.sh")
	statPath := filepath.Join(dir, "mode")
	pathPath := filepath.Join(dir, "where")
	body := "#!/bin/sh\nstat -c %a \"$1\" > " + shellQuote(statPath) + "\nprintf '%s\\n' \"$1\" > " + shellQuote(pathPath) + "\nprintf 'FROM_EDITOR=1\\n' > \"$1\"\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	out, err := Edit([]byte("OLD=1\n"), script)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "FROM_EDITOR=1\n" {
		t.Fatalf("got %q", out)
	}
	mode, err := os.ReadFile(statPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(mode)) != "600" {
		t.Fatalf("mode %q", mode)
	}
	where, err := os.ReadFile(pathPath)
	if err != nil {
		t.Fatal(err)
	}
	p := strings.TrimSpace(string(where))
	if !strings.HasPrefix(p, "/dev/shm/") {
		t.Fatalf("temp path %s, want /dev/shm", p)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("temp file still exists: %v", err)
	}
	parent := filepath.Dir(p)
	if _, err := os.Stat(parent); !os.IsNotExist(err) {
		t.Fatalf("temp dir still exists: %v", err)
	}
}

func TestEditorCommand(t *testing.T) {
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")
	if EditorCommand() != "vim" {
		t.Fatalf("default %q", EditorCommand())
	}
	t.Setenv("VISUAL", "nano")
	if EditorCommand() != "nano" {
		t.Fatalf("visual %q", EditorCommand())
	}
	t.Setenv("EDITOR", "vim -n")
	if EditorCommand() != "vim -n" {
		t.Fatalf("editor should win over visual, got %q", EditorCommand())
	}
}

func TestEditorFalse(t *testing.T) {
	_, err := Edit([]byte("A=1\n"), "false")
	if !os.IsNotExist(err) && !errorsIsEditor(err) {
		t.Fatalf("%v", err)
	}
}

func errorsIsEditor(err error) bool {
	return err != nil && strings.Contains(err.Error(), ErrEditor.Error())
}

func TestEditorMissing(t *testing.T) {
	_, err := Edit([]byte("A=1\n"), "pogo-editor-does-not-exist")
	if err == nil || !strings.Contains(err.Error(), ErrEditorMissing.Error()) {
		t.Fatalf("%v", err)
	}
}

func TestSecureRemoveMissing(t *testing.T) {
	if err := SecureRemove(filepath.Join(t.TempDir(), "nope")); err != nil {
		t.Fatal(err)
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
