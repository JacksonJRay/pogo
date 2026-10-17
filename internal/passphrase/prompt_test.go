package passphrase

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func quietEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, key := range []string{
		"POGO_ASKPASS", "POGO_DEBUG", "POGO_TTY_TIMEOUT", "POGO_PROMPT_TIMEOUT",
		"VSCODE_GIT_IPC_HANDLE", "VSCODE_GIT_IPC_AUTH_TOKEN",
		"DISPLAY", "WAYLAND_DISPLAY", "MIR_SOCKET",
		"XDG_CURRENT_DESKTOP", "XDG_RUNTIME_DIR",
		"SSH_CONNECTION", "SSH_CLIENT", "WSL_DISTRO_NAME", "WSL_INTEROP",
		"TMUX", "STY", "GCR_SSH_ASKPASS_SOCKET",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("PATH", dir)
	t.Cleanup(SetAskpassDirsForTest(nil))
	xdir := filepath.Join(dir, "x11")
	if err := os.MkdirAll(xdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(xdir, "X1"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(SetX11DirForTest(xdir))
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = old
		r.Close()
	})
	return dir
}

func writeExec(t *testing.T, dir, name, body string) string {
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

func withStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	fn()
	w.Close()
	os.Stderr = old
	b, _ := io.ReadAll(r)
	r.Close()
	return string(b)
}

func mustAcquire(t *testing.T, src Source) ([]byte, error) {
	t.Helper()
	type result struct {
		pass []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		pass, err := Acquire(src)
		ch <- result{pass, err}
	}()
	select {
	case res := <-ch:
		return res.pass, res.err
	case <-time.After(5 * time.Second):
		t.Fatal("prompt hung")
		return nil, nil
	}
}

func TestAskpassStdoutAndWhitespace(t *testing.T) {
	dir := quietEnv(t)
	writeExec(t, dir, "ask.sh", "#!/bin/sh\nprintf '  pogo-test-passphrase \\n'\n")
	t.Setenv("POGO_ASKPASS", filepath.Join(dir, "ask.sh"))
	got, err := mustAcquire(t, Source{Prompt: "Enter passphrase: ", File: ".env.enc"})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "  pogo-test-passphrase " {
		t.Fatalf("%q", got)
	}

	writeExec(t, dir, "ask.sh", "#!/bin/sh\nprintf 'keep\\n\\n'\n")
	got, err = mustAcquire(t, Source{Prompt: "Enter passphrase: "})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep\n" {
		t.Fatalf("%q", got)
	}
}

func TestAskpassConfirmMismatchAndMatch(t *testing.T) {
	dir := quietEnv(t)
	count := filepath.Join(dir, "n")
	// PATH is only the temp dir, so the helper uses shell builtins.
	writeExec(t, dir, "ask.sh", "#!/bin/sh\nn=0\n[ -f "+count+" ] && read n < "+count+"\nn=$((n+1))\nprintf '%s' \"$n\" > "+count+"\nif [ \"$n\" = 1 ]; then printf '%s\\n' first-passphrase-aaa; else printf '%s\\n' second-passphrase-bb; fi\n")
	t.Setenv("POGO_ASKPASS", filepath.Join(dir, "ask.sh"))
	got, err := AcquireConfirmed(Source{Prompt: "Enter passphrase: ", File: ".env.enc"}, "Enter passphrase: ", "Enter it again to confirm: ")
	if !errors.Is(err, ErrMismatch) || got != nil {
		t.Fatalf("mismatch %q %v", got, err)
	}

	writeExec(t, dir, "ask.sh", "#!/bin/sh\nprintf '%s\\n' 'pogo-test-passphrase'\n")
	got, err = AcquireConfirmed(Source{File: ".env.enc"}, "Enter passphrase: ", "Enter it again to confirm: ")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "pogo-test-passphrase" {
		t.Fatalf("%q", got)
	}
}

func TestAskpassFailuresWipe(t *testing.T) {
	dir := quietEnv(t)
	script := writeExec(t, dir, "ask.sh", "#!/bin/sh\nexit 3\n")
	t.Setenv("POGO_ASKPASS", script)
	got, err := mustAcquire(t, Source{})
	if err == nil || got != nil {
		t.Fatalf("nonzero %q %v", got, err)
	}

	writeExec(t, dir, "ask.sh", "#!/bin/sh\nexit 0\n")
	got, err = mustAcquire(t, Source{})
	if err == nil || got != nil {
		t.Fatalf("empty %q %v", got, err)
	}

	writeExec(t, dir, "ask.sh", "#!/bin/sh\nprintf '%s\\n' '"+strings.Repeat("a", MaxBytes+2)+"'\n")
	got, err = mustAcquire(t, Source{})
	if !errors.Is(err, ErrTooLong) || got != nil {
		t.Fatalf("long %q %v", got, err)
	}
}

func TestEditorPayloadAndCursorHeader(t *testing.T) {
	quietEnv(t)
	var gotBody []byte
	var gotHeader string
	path := serveUnix(t, func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("x-cursor-git-auth-token")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode("pogo-test-passphrase\n")
	})
	t.Setenv("VSCODE_GIT_IPC_HANDLE", path)
	var got []byte
	var err error
	stderr := withStderr(t, func() {
		got, err = mustAcquire(t, Source{Prompt: "Enter passphrase: ", File: ".env.enc"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr, "Pogo is asking for the .env.enc passphrase in the editor (box at the top of the window).") {
		t.Fatalf("announce %q", stderr)
	}
	if string(got) != "pogo-test-passphrase" {
		t.Fatalf("%q", got)
	}
	if gotHeader != "" {
		t.Fatalf("unexpected cursor header %q", gotHeader)
	}
	var payload struct {
		AskpassType string   `json:"askpassType"`
		Argv        []string `json:"argv"`
	}
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.AskpassType != "https" || len(payload.Argv) < 5 {
		t.Fatalf("%+v", payload)
	}
	if !strings.Contains(strings.ToLower(payload.Argv[2]), "password") || !strings.Contains(payload.Argv[2], ".env.enc") {
		t.Fatalf("placeholder %q", payload.Argv[2])
	}
	wantURL := "https://pogo-passphrase@pogo-passphrase-for-.env.enc.invalid"
	if payload.Argv[4] != wantURL {
		t.Fatalf("host %q", payload.Argv[4])
	}
	if !strings.Contains(payload.Argv[2], "Enter passphrase for .env.enc:") {
		t.Fatalf("placeholder %q", payload.Argv[2])
	}

	t.Setenv("VSCODE_GIT_IPC_AUTH_TOKEN", "cursor-token-test")
	gotHeader = ""
	gotBody = nil
	got, err = mustAcquire(t, Source{Prompt: "Enter passphrase: ", File: ".env.enc"})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "pogo-test-passphrase" {
		t.Fatalf("%q", got)
	}
	if gotHeader != "cursor-token-test" {
		t.Fatalf("header %q", gotHeader)
	}
	var cursor map[string]any
	if err := json.Unmarshal(gotBody, &cursor); err != nil {
		t.Fatal(err)
	}
	if _, ok := cursor["argv"]; ok {
		t.Fatalf("cursor payload included argv: %s", gotBody)
	}
	if cursor["askpassType"] != "https" || cursor["host"] != "https://pogo-passphrase@pogo-passphrase-for-.env.enc.invalid" {
		t.Fatalf("%s", gotBody)
	}
	req, _ := cursor["request"].(string)
	if !strings.Contains(strings.ToLower(req), "password") || !strings.Contains(req, ".env.enc") {
		t.Fatalf("request %q", req)
	}
}

func TestEditorNewAndConfirmHosts(t *testing.T) {
	quietEnv(t)
	var mu sync.Mutex
	var hosts []string
	var prompts []string
	var decodeErr string
	path := serveUnix(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload struct {
			Argv []string `json:"argv"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			mu.Lock()
			decodeErr = err.Error()
			mu.Unlock()
		}
		mu.Lock()
		if len(payload.Argv) >= 5 {
			prompts = append(prompts, payload.Argv[2])
			hosts = append(hosts, payload.Argv[4])
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode("pogo-test-passphrase\n")
	})
	t.Setenv("VSCODE_GIT_IPC_HANDLE", path)
	got, err := AcquireConfirmed(Source{File: ".env.enc"}, "New passphrase for .env.enc: ", "Confirm new passphrase for .env.enc: ")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "pogo-test-passphrase" {
		t.Fatalf("%q", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if decodeErr != "" {
		t.Fatal(decodeErr)
	}
	if len(hosts) != 2 {
		t.Fatalf("hosts %v", hosts)
	}
	if hosts[0] != "https://pogo-passphrase@pogo-new-passphrase-for-.env.enc.invalid" {
		t.Fatalf("new host %q", hosts[0])
	}
	if hosts[1] != "https://pogo-passphrase@pogo-confirm-passphrase-for-.env.enc.invalid" {
		t.Fatalf("confirm host %q", hosts[1])
	}
	if !strings.Contains(prompts[0], "New passphrase for .env.enc:") || !strings.Contains(prompts[1], "Confirm new passphrase for .env.enc:") {
		t.Fatalf("prompts %q", prompts)
	}
}

func TestEmptyEditorDoesNotCallDesktop(t *testing.T) {
	dir := quietEnv(t)
	log := filepath.Join(dir, "called")
	writeExec(t, dir, "zenity", "#!/bin/sh\necho called >> "+log+"\nprintf '%s\\n' 'should-not-run'\n")
	t.Setenv("DISPLAY", ":1")
	t.Setenv("XDG_CURRENT_DESKTOP", "")
	path := serveUnix(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	t.Setenv("VSCODE_GIT_IPC_HANDLE", path)
	got, err := mustAcquire(t, Source{Prompt: "Enter passphrase: "})
	if !errors.Is(err, ErrCancelled) || got != nil {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatal("desktop helper ran after an empty editor response")
	}
}

func TestEditorNon200FallsThrough(t *testing.T) {
	dir := quietEnv(t)
	writeExec(t, dir, "ssh-askpass", "#!/bin/sh\nprintf '%s\\n' 'from-desktop-helper'\n")
	t.Setenv("DISPLAY", ":1")
	path := serveUnix(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	})
	t.Setenv("VSCODE_GIT_IPC_HANDLE", path)
	got, err := mustAcquire(t, Source{})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "from-desktop-helper" {
		t.Fatalf("%q", got)
	}
}

func TestNoBackendAndEnvironments(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, dir string)
		want  []string
	}{
		{
			name: "no backends",
			setup: func(t *testing.T, dir string) {
				t.Setenv("PATH", "")
			},
			want: []string{NoPromptSummary, "POGO_ASKPASS not set", "editor socket not set", "no display", "Last resort", AgentLine},
		},
		{
			name: "empty PATH",
			setup: func(t *testing.T, dir string) {
				t.Setenv("PATH", "")
				t.Setenv("HOME", "")
			},
			want: []string{"POGO_ASKPASS not set", "no display", AgentLine},
		},
		{
			name: "missing socket",
			setup: func(t *testing.T, dir string) {
				t.Setenv("VSCODE_GIT_IPC_HANDLE", filepath.Join(dir, "missing.sock"))
			},
			want: []string{"editor socket not found"},
		},
		{
			name: "refused socket",
			setup: func(t *testing.T, dir string) {
				// Closing a net.Listener unlinks the socket, which is "not
				// found". Bind without listen so the file stays and dial
				// returns connection refused.
				sock := filepath.Join(dir, "dead.sock")
				fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM, 0)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { unix.Close(fd) })
				if err := unix.Bind(fd, &unix.SockaddrUnix{Name: sock}); err != nil {
					t.Fatal(err)
				}
				t.Setenv("VSCODE_GIT_IPC_HANDLE", sock)
			},
			want: []string{"editor socket connection refused"},
		},
		{
			name: "display helper cannot open display",
			setup: func(t *testing.T, dir string) {
				writeExec(t, dir, "zenity", "#!/bin/sh\necho 'cannot open display: :99' >&2\nexit 1\n")
				if err := os.WriteFile(filepath.Join(dir, "x11", "X99"), []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
				t.Setenv("DISPLAY", ":99")
			},
			want: []string{"zenity could not open a display", NoPromptSummary},
		},
		{
			name: "ssh without display",
			setup: func(t *testing.T, dir string) {
				t.Setenv("SSH_CONNECTION", "203.0.113.5 40000 203.0.113.8 22")
			},
			want: []string{"no display (SSH session)"},
		},
		{
			name: "wsl without wslg",
			setup: func(t *testing.T, dir string) {
				t.Setenv("WSL_DISTRO_NAME", "Ubuntu")
			},
			want: []string{"no display (WSL without WSLg)"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := quietEnv(t)
			tc.setup(t, dir)
			start := time.Now()
			got, err := mustAcquire(t, Source{Prompt: "Enter passphrase: ", File: ".env.enc"})
			if time.Since(start) > 4*time.Second {
				t.Fatal("hung")
			}
			if got != nil {
				t.Fatalf("passphrase returned %q", got)
			}
			var np *NoPromptError
			if !errors.As(err, &np) {
				t.Fatalf("%v", err)
			}
			text := err.Error()
			if !strings.Contains(text, NoPromptSummary) || !strings.Contains(text, AgentLine) {
				t.Fatalf("%s", text)
			}
			for _, part := range tc.want {
				if !strings.Contains(text, part) {
					t.Errorf("missing %q in %s", part, text)
				}
			}
			// A listening socket or an installed helper is a backend for
			// doctor even when the dialog itself then fails. The cases
			// below have neither.
			switch tc.name {
			case "display helper cannot open display":
			default:
				d := Diagnose()
				if d.Available {
					t.Fatalf("doctor available: %+v", d)
				}
				if codeText := d.Report(); !strings.Contains(codeText, NoPromptSummary) {
					t.Fatalf("doctor %s", codeText)
				}
			}
		})
	}
}

func TestFlatpakSocketAndAmbiguousRuntime(t *testing.T) {
	dir := quietEnv(t)
	runtime := filepath.Join(dir, "runtime")
	if err := os.MkdirAll(runtime, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", runtime)
	sock := filepath.Join(runtime, "vscode-git-abc.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode("from-flatpak-socket")
	})}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	t.Setenv("VSCODE_GIT_IPC_HANDLE", "/host/hidden/vscode-git-abc.sock")
	got, err := mustAcquire(t, Source{File: ".env.enc"})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "from-flatpak-socket" {
		t.Fatalf("%q", got)
	}

	// A second socket with a different name makes the fallback ambiguous
	// when the basename is also missing.
	other := filepath.Join(runtime, "vscode-git-other.sock")
	ln2, err := net.Listen("unix", other)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln2.Close() })
	t.Setenv("VSCODE_GIT_IPC_HANDLE", "/host/hidden/does-not-match.sock")
	_, err = mustAcquire(t, Source{})
	if err == nil || !strings.Contains(err.Error(), "several vscode-git sockets") {
		t.Fatalf("%v", err)
	}
}

func TestDesktopPreferenceAndCancel(t *testing.T) {
	dir := quietEnv(t)
	log := filepath.Join(dir, "which")
	// gcr4 only runs when GNOME's relay socket exists.
	ln, err := net.Listen("unix", filepath.Join(dir, "gcr.sock"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	t.Setenv("GCR_SSH_ASKPASS_SOCKET", filepath.Join(dir, "gcr.sock"))
	writeExec(t, dir, "gcr4-ssh-askpass", "#!/bin/sh\necho gcr4 >> "+log+"\nprintf '%s\\n' 'from-gcr'\n")
	writeExec(t, dir, "ksshaskpass", "#!/bin/sh\necho kssh >> "+log+"\nprintf '%s\\n' 'from-kde'\n")
	t.Setenv("DISPLAY", ":1")
	t.Setenv("XDG_CURRENT_DESKTOP", "ubuntu:GNOME")
	got, err := mustAcquire(t, Source{File: ".env.enc"})
	if err != nil || string(got) != "from-gcr" {
		t.Fatalf("%q %v", got, err)
	}

	os.Remove(log)
	t.Setenv("XDG_CURRENT_DESKTOP", "KDE")
	got, err = mustAcquire(t, Source{})
	if err != nil || string(got) != "from-kde" {
		t.Fatalf("%q %v", got, err)
	}

	// Unknown desktop: askpass protocol before zenity. Display failure falls
	// through; a plain non-zero exit is cancel and stops. Remove the helpers
	// the earlier cases left on PATH so ssh-askpass is the first match.
	os.Remove(filepath.Join(dir, "gcr4-ssh-askpass"))
	os.Remove(filepath.Join(dir, "ksshaskpass"))
	os.Remove(log)
	writeExec(t, dir, "ssh-askpass", "#!/bin/sh\necho ssh >> "+log+"\necho 'cannot open display: :1' >&2\nexit 1\n")
	writeExec(t, dir, "zenity", "#!/bin/sh\necho zenity >> "+log+"\nprintf '%s\\n' 'from-zenity'\n")
	t.Setenv("XDG_CURRENT_DESKTOP", "")
	got, err = mustAcquire(t, Source{})
	if err != nil || string(got) != "from-zenity" {
		t.Fatalf("fallthrough %q %v", got, err)
	}
	which, _ := os.ReadFile(log)
	if !bytes.Contains(which, []byte("ssh")) || !bytes.Contains(which, []byte("zenity")) {
		t.Fatalf("order %s", which)
	}

	os.Remove(log)
	writeExec(t, dir, "ssh-askpass", "#!/bin/sh\necho ssh >> "+log+"\nexit 1\n")
	got, err = mustAcquire(t, Source{})
	if !errors.Is(err, ErrCancelled) || got != nil {
		t.Fatalf("cancel %q %v", got, err)
	}
	which, _ = os.ReadFile(log)
	if bytes.Contains(which, []byte("zenity")) {
		t.Fatalf("cancel fell through: %s", which)
	}
}

func TestNixOSPathNotHardcodedStore(t *testing.T) {
	dir := quietEnv(t)
	// PATH only, no libexec directories. This is the NixOS and Alpine case.
	writeExec(t, dir, "ssh-askpass", "#!/bin/sh\nprintf '%s\\n' 'from-path'\n")
	t.Setenv("DISPLAY", ":1")
	t.Setenv("XDG_CURRENT_DESKTOP", "")
	got, err := mustAcquire(t, Source{})
	if err != nil || string(got) != "from-path" {
		t.Fatalf("%q %v", got, err)
	}

	var dirs string
	for _, h := range defaultHelpers() {
		dirs += strings.Join(h.dirs, "\n") + "\n"
	}
	for _, needle := range []string{"/usr/libexec", "/usr/bin", "/run/current-system/sw/bin", "/run/current-system/sw/libexec"} {
		if !strings.Contains(dirs, needle) {
			t.Errorf("search paths missing %s", needle)
		}
	}
	if strings.Contains(dirs, "/nix/store") {
		t.Fatal("hardcoded a nix store path")
	}
}

func TestDebugAndDoctorAskpass(t *testing.T) {
	dir := quietEnv(t)
	stderr := withStderr(t, func() {
		t.Setenv("POGO_DEBUG", "1")
		_, _ = mustAcquire(t, Source{})
	})
	if !strings.Contains(stderr, "pogo: stdin is not a terminal") || !strings.Contains(stderr, "pogo: POGO_ASKPASS not set") {
		t.Fatalf("debug %q", stderr)
	}

	script := writeExec(t, dir, "ask.sh", "#!/bin/sh\nprintf '%s\\n' 'secret-value'\n")
	t.Setenv("POGO_ASKPASS", script)
	d := Diagnose()
	if !d.Available || d.Backend != "POGO_ASKPASS" {
		t.Fatalf("%+v", d)
	}
	if strings.Contains(d.Report(), "secret-value") {
		t.Fatalf("doctor printed a passphrase: %s", d.Report())
	}
	got, err := mustAcquire(t, Source{})
	if err != nil || string(got) != "secret-value" {
		t.Fatalf("%q %v", got, err)
	}
}

func TestAskpassBeatsEditor(t *testing.T) {
	dir := quietEnv(t)
	hits := 0
	path := serveUnix(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		_ = json.NewEncoder(w).Encode("from-editor")
	})
	t.Setenv("VSCODE_GIT_IPC_HANDLE", path)
	writeExec(t, dir, "ask.sh", "#!/bin/sh\nprintf '%s\\n' 'from-askpass'\n")
	t.Setenv("POGO_ASKPASS", filepath.Join(dir, "ask.sh"))
	got, err := mustAcquire(t, Source{})
	if err != nil || string(got) != "from-askpass" || hits != 0 {
		t.Fatalf("%q %v hits=%d", got, err, hits)
	}
}

func TestLibexecWhenPathEmpty(t *testing.T) {
	dir := quietEnv(t)
	lib := filepath.Join(dir, "libexec")
	writeExec(t, lib, "gcr4-ssh-askpass", "#!/bin/sh\nprintf '%s\\n' 'from-libexec'\n")
	t.Cleanup(SetAskpassDirsForTest([]string{lib}))
	t.Setenv("PATH", "")
	t.Setenv("DISPLAY", ":1")
	// The libexec binary is named gcr4-ssh-askpass, which Pogo runs only
	// when GNOME's relay socket exists. The script itself ignores the socket.
	ln, err := net.Listen("unix", filepath.Join(dir, "gcr.sock"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	t.Setenv("GCR_SSH_ASKPASS_SOCKET", ln.Addr().String())
	got, err := mustAcquire(t, Source{})
	if err != nil || string(got) != "from-libexec" {
		t.Fatalf("%q %v", got, err)
	}
}

func TestGCRRelaySkippedWithoutSocket(t *testing.T) {
	dir := quietEnv(t)
	log := filepath.Join(dir, "which")
	writeExec(t, dir, "gcr4-ssh-askpass", "#!/bin/sh\necho gcr4 >> "+log+"\nprintf '%s\\n' 'from-gcr'\n")
	writeExec(t, dir, "zenity", "#!/bin/sh\necho zenity >> "+log+"\nprintf '%s\\n' 'from-zenity'\n")
	t.Setenv("DISPLAY", ":1")
	t.Setenv("XDG_CURRENT_DESKTOP", "GNOME")
	got, err := mustAcquire(t, Source{File: ".env.enc"})
	if err != nil || string(got) != "from-zenity" {
		t.Fatalf("skip %q %v", got, err)
	}
	which, _ := os.ReadFile(log)
	if bytes.Contains(which, []byte("gcr4")) || !bytes.Contains(which, []byte("zenity")) {
		t.Fatalf("ran %s", which)
	}
	d := Diagnose()
	if !d.Available || d.Backend != "zenity" {
		t.Fatalf("doctor %+v", d)
	}
	if !strings.Contains(d.Report(), "GCR askpass socket is not available") {
		t.Fatalf("doctor %s", d.Report())
	}

	// A live socket makes the relay eligible. A relay that still cannot
	// connect falls through; it is not a cancel.
	sock := filepath.Join(dir, "gcr.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	t.Setenv("GCR_SSH_ASKPASS_SOCKET", sock)
	writeExec(t, dir, "gcr4-ssh-askpass", "#!/bin/sh\necho gcr4 >> "+log+"\necho 'gcr4-ssh-askpass: this program is not meant to be run directly' >&2\nexit 2\n")
	os.Remove(log)
	got, err = mustAcquire(t, Source{})
	if err != nil || string(got) != "from-zenity" {
		t.Fatalf("fallthrough %q %v", got, err)
	}
	which, _ = os.ReadFile(log)
	if !bytes.Contains(which, []byte("gcr4")) || !bytes.Contains(which, []byte("zenity")) {
		t.Fatalf("order %s", which)
	}

	writeExec(t, dir, "gcr4-ssh-askpass", "#!/bin/sh\necho gcr4 >> "+log+"\nprintf '%s\\n' 'from-gcr-socket'\n")
	os.Remove(log)
	got, err = mustAcquire(t, Source{})
	if err != nil || string(got) != "from-gcr-socket" {
		t.Fatalf("socket %q %v", got, err)
	}
}

func TestEditorResponseTimeoutDoesNotFallThrough(t *testing.T) {
	dir := quietEnv(t)
	log := filepath.Join(dir, "called")
	writeExec(t, dir, "zenity", "#!/bin/sh\necho called >> "+log+"\nprintf '%s\\n' 'from-desktop'\n")
	t.Setenv("DISPLAY", ":1")
	t.Cleanup(SetPromptTimeoutsForTest(200*time.Millisecond, 300*time.Millisecond))
	sock := filepath.Join(dir, "hang.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		time.Sleep(time.Minute)
		c.Close()
	}()
	t.Setenv("VSCODE_GIT_IPC_HANDLE", sock)
	start := time.Now()
	got, err := mustAcquire(t, Source{Prompt: "Enter passphrase: ", File: ".env.enc"})
	if time.Since(start) > 4*time.Second {
		t.Fatal("hung")
	}
	if got != nil {
		t.Fatalf("passphrase returned %q", got)
	}
	var timed *EditorTimeoutError
	if !errors.As(err, &timed) {
		t.Fatalf("%v", err)
	}
	text := err.Error()
	if !strings.Contains(text, EditorTimeoutSummary) || !strings.Contains(text, "Ways forward, in order:") || !strings.Contains(text, AgentLine) {
		t.Fatalf("%s", text)
	}
	if strings.Contains(text, NoPromptSummary) {
		t.Fatalf("fell through to the no-prompt message:\n%s", text)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatal("desktop helper ran after an editor response timeout")
	}
}

func TestHelperTimeoutIsNotCancel(t *testing.T) {
	dir := quietEnv(t)
	t.Cleanup(SetPromptTimeoutsForTest(200*time.Millisecond, 300*time.Millisecond))
	log := filepath.Join(dir, "called")
	writeExec(t, dir, "ask.sh", "#!/bin/sh\n/bin/sleep 30\n")
	writeExec(t, dir, "zenity", "#!/bin/sh\necho zenity >> "+log+"\nprintf '%s\\n' 'from-zenity'\n")
	t.Setenv("POGO_ASKPASS", filepath.Join(dir, "ask.sh"))
	t.Setenv("DISPLAY", ":1")
	start := time.Now()
	got, err := mustAcquire(t, Source{File: ".env.enc"})
	if time.Since(start) > 4*time.Second {
		t.Fatal("hung")
	}
	if got != nil {
		t.Fatalf("passphrase returned %q", got)
	}
	var timed *PromptTimeoutError
	if !errors.As(err, &timed) || timed.What != "POGO_ASKPASS" {
		t.Fatalf("%v", err)
	}
	text := err.Error()
	if !strings.Contains(text, "pogo: the POGO_ASKPASS prompt got no answer within the timeout") || !strings.Contains(text, "Ways forward, in order:") || !strings.Contains(text, AgentLine) {
		t.Fatalf("%s", text)
	}
	if strings.Contains(text, "cancelled") || strings.Contains(text, NoPromptSummary) {
		t.Fatalf("%s", text)
	}
	if _, statErr := os.Stat(log); !os.IsNotExist(statErr) {
		t.Fatal("desktop helper ran after POGO_ASKPASS timed out")
	}

	t.Setenv("POGO_ASKPASS", "")
	writeExec(t, dir, "ssh-askpass", "#!/bin/sh\necho ssh >> "+log+"\n/bin/sleep 30\n")
	start = time.Now()
	got, err = mustAcquire(t, Source{})
	if time.Since(start) > 4*time.Second {
		t.Fatal("hung")
	}
	if got != nil {
		t.Fatalf("desktop passphrase %q", got)
	}
	timed = nil
	if !errors.As(err, &timed) || timed.What != "ssh-askpass" {
		t.Fatalf("%v", err)
	}
	text = err.Error()
	if !strings.Contains(text, "pogo: the ssh-askpass prompt got no answer within the timeout") || !strings.Contains(text, "Ways forward, in order:") || !strings.Contains(text, AgentLine) {
		t.Fatalf("%s", text)
	}
	if strings.Contains(text, "cancelled") {
		t.Fatalf("%s", text)
	}
	which, _ := os.ReadFile(log)
	if bytes.Contains(which, []byte("zenity")) {
		t.Fatalf("timeout fell through: %s", which)
	}

	os.Remove(log)
	writeExec(t, dir, "ssh-askpass", "#!/bin/sh\necho ssh >> "+log+"\nexit 1\n")
	got, err = mustAcquire(t, Source{})
	if !errors.Is(err, ErrCancelled) || got != nil {
		t.Fatalf("cancel %q %v", got, err)
	}
	if strings.Contains(err.Error(), "got no answer") {
		t.Fatalf("cancel looked like a timeout: %v", err)
	}
	which, _ = os.ReadFile(log)
	if bytes.Contains(which, []byte("zenity")) {
		t.Fatalf("cancel fell through: %s", which)
	}
}

func TestDisplaySocketRequired(t *testing.T) {
	dir := quietEnv(t)
	log := filepath.Join(dir, "called")
	writeExec(t, dir, "zenity", "#!/bin/sh\necho called >> "+log+"\nprintf '%s\\n' 'from-zenity'\n")

	t.Setenv("DISPLAY", ":99")
	got, err := mustAcquire(t, Source{})
	if got != nil {
		t.Fatalf("stale display returned %q", got)
	}
	var np *NoPromptError
	if !errors.As(err, &np) || !strings.Contains(err.Error(), NoXSocketCheck) {
		t.Fatalf("%v", err)
	}
	if _, statErr := os.Stat(log); !os.IsNotExist(statErr) {
		t.Fatal("zenity ran without an X socket")
	}
	d := Diagnose()
	if d.Available || !strings.Contains(d.Report(), NoXSocketCheck) {
		t.Fatalf("doctor %+v %s", d, d.Report())
	}

	// SSH forwarding uses a TCP display. Do not require a local socket.
	os.Remove(log)
	t.Setenv("DISPLAY", "localhost:10.0")
	got, err = mustAcquire(t, Source{})
	if err != nil || string(got) != "from-zenity" {
		t.Fatalf("tcp display %q %v", got, err)
	}

	// A relative Wayland name is the socket under XDG_RUNTIME_DIR.
	os.Remove(log)
	runtime := filepath.Join(dir, "runtime")
	if err := os.MkdirAll(runtime, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtime, "wayland-0"), []byte("w"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DISPLAY", "")
	t.Setenv("XDG_RUNTIME_DIR", runtime)
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	got, err = mustAcquire(t, Source{})
	if err != nil || string(got) != "from-zenity" {
		t.Fatalf("wayland %q %v", got, err)
	}

	os.Remove(log)
	t.Setenv("WAYLAND_DISPLAY", "wayland-missing")
	got, err = mustAcquire(t, Source{})
	if got != nil || !errors.As(err, &np) || !strings.Contains(err.Error(), NoXSocketCheck) {
		t.Fatalf("missing wayland %q %v", got, err)
	}
	if _, statErr := os.Stat(log); !os.IsNotExist(statErr) {
		t.Fatal("zenity ran without a wayland socket")
	}

	abs := filepath.Join(dir, "wayland.sock")
	if err := os.WriteFile(abs, []byte("w"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WAYLAND_DISPLAY", abs)
	got, err = mustAcquire(t, Source{})
	if err != nil || string(got) != "from-zenity" {
		t.Fatalf("absolute wayland %q %v", got, err)
	}
}

func TestPromptTimeoutEnv(t *testing.T) {
	quietEnv(t)
	t.Cleanup(SetPromptTimeoutsForTest(time.Second, 30*time.Second))
	t.Setenv("POGO_PROMPT_TIMEOUT", "1")
	if dialogTimeout() != time.Second {
		t.Fatalf("%s", dialogTimeout())
	}
	t.Setenv("POGO_PROMPT_TIMEOUT", "nope")
	if dialogTimeout() != 30*time.Second {
		t.Fatalf("invalid %s", dialogTimeout())
	}
	t.Setenv("POGO_PROMPT_TIMEOUT", "")
	if dialogTimeout() != 30*time.Second {
		t.Fatalf("unset %s", dialogTimeout())
	}
	if n, ok := localXNumber(":1"); !ok || n != "1" {
		t.Fatalf(":1 %q %v", n, ok)
	}
	if n, ok := localXNumber("unix:2.0"); !ok || n != "2" {
		t.Fatalf("unix %q %v", n, ok)
	}
	if _, ok := localXNumber("localhost:10.0"); ok {
		t.Fatal("tcp display was treated as local")
	}
	if editorURL(".env.enc", KindCurrent) != "https://pogo-passphrase@pogo-passphrase-for-.env.enc.invalid" {
		t.Fatal(editorURL(".env.enc", KindCurrent))
	}
	if editorURL(".env.enc", KindNew) != "https://pogo-passphrase@pogo-new-passphrase-for-.env.enc.invalid" {
		t.Fatal(editorURL(".env.enc", KindNew))
	}
	if editorURL(".env.enc", KindConfirm) != "https://pogo-passphrase@pogo-confirm-passphrase-for-.env.enc.invalid" {
		t.Fatal(editorURL(".env.enc", KindConfirm))
	}
	if strings.Contains(editorURL("my file", KindCurrent), " ") || !strings.HasSuffix(editorURL("my file", KindCurrent), ".invalid") {
		t.Fatal(editorURL("my file", KindCurrent))
	}
}

func TestForbiddenBackends(t *testing.T) {
	for _, name := range []string{"prompt.go", "editor_ipc.go", "desktop.go", "diagnose.go", "passphrase.go"} {
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{"pkexec", "systemd-ask-password", "askpass.sh", "/etc/os-release"} {
			if bytes.Contains(b, []byte(bad)) {
				t.Errorf("%s contains %s", name, bad)
			}
		}
	}
}

func TestDesktopFamily(t *testing.T) {
	if desktopFamily("ubuntu:GNOME") != "gnome" || desktopFamily("pop:GNOME") != "gnome" {
		t.Fatal(desktopFamily("ubuntu:GNOME"))
	}
	if desktopFamily("KDE") != "kde" || desktopFamily("plasma") != "kde" {
		t.Fatal("kde")
	}
	if desktopFamily("") != "" || desktopFamily("sway") != "" {
		t.Fatal("unknown")
	}
}

func serveUnix(t *testing.T, h http.HandlerFunc) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "vscode-git-test.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: h}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return path
}
