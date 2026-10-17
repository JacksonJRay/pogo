package editfile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"pogo/internal/passphrase"
)

func quietEditorEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"POGO_EDITOR", "POGO_EDIT_TIMEOUT", "POGO_DEBUG",
		"EDITOR", "VISUAL", "TERM_PROGRAM",
		"VSCODE_IPC_HOOK_CLI", "VSCODE_GIT_ASKPASS_NODE",
		"DISPLAY", "WAYLAND_DISPLAY", "MIR_SOCKET",
		"SSH_CONNECTION", "SSH_CLIENT", "WSL_DISTRO_NAME", "WSL_INTEROP",
		"XDG_RUNTIME_DIR",
	} {
		t.Setenv(key, "")
	}
}

func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSelectPOGOEditor(t *testing.T) {
	quietEditorEnv(t)
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	t.Setenv("EDITOR", "vim")
	script := writeScript(t, dir, "my-edit", "#!/bin/sh\nexit 0\n")
	t.Setenv("POGO_EDITOR", script+" --wait")
	choice, err := SelectTTY(false)
	if err != nil {
		t.Fatal(err)
	}
	if choice.Source != "POGO_EDITOR" || len(choice.Argv) != 2 || choice.Argv[0] != script || choice.Argv[1] != "--wait" {
		t.Fatalf("%+v", choice)
	}
}

func TestSelectRemoteCLI(t *testing.T) {
	quietEditorEnv(t)
	root := t.TempDir()
	node := writeScript(t, root, "node", "#!/bin/sh\nexit 0\n")
	cli := writeScript(t, root, "bin/remote-cli/cursor", "#!/bin/sh\nfile=\nfor a in \"$@\"; do file=\"$a\"; done\nsleep 0.2\nprintf 'from-cursor\\n' > \"$file\"\n")
	writeScript(t, root, "bin/remote-cli/code", "#!/bin/sh\nexit 1\n")
	t.Setenv("PATH", filepath.Join(root, "empty"))
	t.Setenv("VSCODE_GIT_ASKPASS_NODE", node)
	t.Setenv("VSCODE_IPC_HOOK_CLI", filepath.Join(root, "hook"))
	t.Setenv("TERM_PROGRAM", "cursor")
	t.Setenv("EDITOR", "vim")
	choice, err := SelectTTY(false)
	if err != nil {
		t.Fatal(err)
	}
	if choice.Source != "remote-cli" || !choice.Announce || choice.Argv[0] != cli || choice.Argv[1] != "--wait" {
		t.Fatalf("%+v", choice)
	}
	out, err := Edit(nil, choice.CommandLine())
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "from-cursor\n" {
		t.Fatalf("%q", out)
	}
}

func TestSelectRemoteCLIOnPATH(t *testing.T) {
	quietEditorEnv(t)
	dir := t.TempDir()
	writeScript(t, dir, "code-server", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", dir)
	t.Setenv("VSCODE_IPC_HOOK_CLI", "/tmp/code-server-hook")
	t.Setenv("TERM_PROGRAM", "code-server")
	t.Setenv("EDITOR", "nano")
	choice, err := SelectTTY(false)
	if err != nil {
		t.Fatal(err)
	}
	if choice.Source != "remote-cli" || filepath.Base(choice.Argv[0]) != "code-server" || choice.Argv[1] != "--wait" {
		t.Fatalf("%+v", choice)
	}
}

func TestSelectSkipsTerminalEditorWithoutTTY(t *testing.T) {
	quietEditorEnv(t)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("EDITOR", "vim -n")
	t.Setenv("VISUAL", "nano")
	_, err := SelectTTY(false)
	var ne *NoEditorError
	if !errors.As(err, &ne) {
		t.Fatalf("%v", err)
	}
	text := err.Error()
	for _, part := range []string{NoEditorSummary, "EDITOR is vim, which needs a terminal", "VISUAL is nano, which needs a terminal", "POGO_EDITOR not set", "VSCODE_IPC_HOOK_CLI not set", "Run `pogo edit` in your own terminal", "POGO_EDITOR", "integrated terminal", AgentEditLine} {
		if !strings.Contains(text, part) {
			t.Errorf("missing %q\n%s", part, text)
		}
	}
}

func TestSelectScriptEditorWithoutTTY(t *testing.T) {
	quietEditorEnv(t)
	t.Setenv("EDITOR", "/tmp/write-env.sh")
	t.Setenv("VISUAL", "vim")
	choice, err := SelectTTY(false)
	if err != nil {
		t.Fatal(err)
	}
	if choice.Source != "EDITOR" || choice.Argv[0] != "/tmp/write-env.sh" {
		t.Fatalf("%+v", choice)
	}
}

func TestSelectGUIRequiresWait(t *testing.T) {
	quietEditorEnv(t)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("EDITOR", "code")
	t.Setenv("VISUAL", "xdg-open")
	_, err := SelectTTY(false)
	if err == nil || !strings.Contains(err.Error(), "without a wait flag") || !strings.Contains(err.Error(), "xdg-open") {
		t.Fatalf("%v", err)
	}
	t.Setenv("EDITOR", "code --wait")
	choice, err := SelectTTY(false)
	if err != nil || choice.Argv[0] != "code" || choice.Argv[1] != "--wait" || !choice.Announce {
		t.Fatalf("%v %+v", err, choice)
	}
}

func TestSelectDesktopGUI(t *testing.T) {
	quietEditorEnv(t)
	dir := t.TempDir()
	sock := filepath.Join(dir, "X1")
	if err := os.WriteFile(sock, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "bin")
	writeScript(t, bin, "gedit", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", bin)
	t.Setenv("DISPLAY", ":1")
	t.Setenv("EDITOR", "vim")
	t.Cleanup(passphrase.SetX11DirForTest(dir))
	choice, err := SelectTTY(false)
	if err != nil {
		t.Fatal(err)
	}
	if choice.Source != "desktop" || filepath.Base(choice.Argv[0]) != "gedit" || !strings.Contains(strings.Join(choice.Argv, " "), "-s") {
		t.Fatalf("%+v", choice)
	}
}

func TestSelectTTYUsesVim(t *testing.T) {
	quietEditorEnv(t)
	choice, err := SelectTTY(true)
	if err != nil || len(choice.Argv) != 1 || choice.Argv[0] != "vim" {
		t.Fatalf("%v %+v", err, choice)
	}
}

func TestEditTimeoutRemovesTemp(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("POGO_EDIT_TIMEOUT", "1")
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	old := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = old; r.Close() }()
	dir := t.TempDir()
	marker := filepath.Join(dir, "where")
	script := writeScript(t, dir, "sleep-edit", "#!/bin/sh\nprintf '%s\\n' \"$1\" > "+shellQuote(marker)+"\nsleep 30\n")
	errc := make(chan error, 1)
	go func() {
		_, err := Edit([]byte("A=1\n"), script)
		errc <- err
	}()
	var path string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(marker)
		if err == nil {
			path = strings.TrimSpace(string(b))
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if path == "" {
		t.Fatal("editor did not start")
	}
	select {
	case err := <-errc:
		if !errors.Is(err, ErrTimeout) {
			t.Fatalf("%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for edit timeout")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("temp file remains: %v", err)
	}
}

func TestEditSignalRemovesTemp(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("POGO_EDIT_TIMEOUT", "")
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	old := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = old; r.Close() }()

	dir := t.TempDir()
	marker := filepath.Join(dir, "where")
	script := writeScript(t, dir, "sleep-edit", "#!/bin/sh\nprintf '%s\\n' \"$1\" > "+shellQuote(marker)+"\nsleep 30\n")
	errc := make(chan error, 1)
	go func() {
		_, err := Edit([]byte("A=1\n"), script)
		errc <- err
	}()
	var path string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(marker)
		if err == nil {
			path = strings.TrimSpace(string(b))
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if path == "" {
		t.Fatal("editor did not start")
	}
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errc:
		if !errors.Is(err, ErrInterrupted) {
			t.Fatalf("%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for signal")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("temp file remains: %v", err)
	}
}

func TestEditFailureRemovesTemp(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	dir := t.TempDir()
	marker := filepath.Join(dir, "where")
	script := writeScript(t, dir, "bad-edit", "#!/bin/sh\nprintf '%s\\n' \"$1\" > "+shellQuote(marker)+"\nexit 1\n")
	_, err := Edit([]byte("A=1\n"), script)
	if !errors.Is(err, ErrEditor) {
		t.Fatalf("%v", err)
	}
	where, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	path := strings.TrimSpace(string(where))
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("temp file remains: %v", err)
	}
}

func TestXDGRuntimeDirPreferred(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", base)
	if TempBase() != base {
		t.Fatalf("base %s", TempBase())
	}
	dir := t.TempDir()
	script := writeScript(t, dir, "ed.sh", "#!/bin/sh\nprintf '%s\\n' \"$1\"\n")
	// The script prints the path on stdout, which Edit does not capture.
	// Stat the runtime dir for a pogo- directory during the run by writing it.
	marker := filepath.Join(dir, "where")
	script = writeScript(t, dir, "ed.sh", "#!/bin/sh\nprintf '%s\\n' \"$1\" > "+shellQuote(marker)+"\n")
	if _, err := Edit([]byte("A=1\n"), script); err != nil {
		t.Fatal(err)
	}
	where, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	path := strings.TrimSpace(string(where))
	if !strings.HasPrefix(path, base+string(os.PathSeparator)) {
		t.Fatalf("path %s", where)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("temp file remains after success: %v", err)
	}
}
