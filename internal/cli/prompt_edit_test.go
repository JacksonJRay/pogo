package cli

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"pogo/internal/editfile"
	"pogo/internal/filefmt"
	"pogo/internal/passphrase"
)

func closedStdin(t *testing.T) {
	t.Helper()
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
}

func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func writeAskpass(t *testing.T, dir, log, kind string) string {
	t.Helper()
	body := "#!/bin/sh\nprintf '%s\\n' \"$1\" >> " + shQuote(log) + "\nn=$(wc -l < " + shQuote(log) + ")\nn=$(printf '%s' \"$n\" | tr -d ' ')\n"
	switch kind {
	case "mismatch":
		body += "if [ \"$n\" -eq 1 ]; then printf '%s\\n' 'pogo-test-passphrase'; else printf '%s\\n' 'other-passphrase-value'; fi\n"
	case "cancel":
		body += "exit 1\n"
	case "cancel2":
		body += "if [ \"$n\" -eq 1 ]; then printf '%s\\n' 'pogo-test-passphrase'; else exit 1; fi\n"
	case "short":
		body += "printf '%s\\n' 'short'\n"
	case "sequence":
		body += "case \"$n\" in\n1) printf '%s\\n' 'pogo-test-passphrase' ;;\n2) printf '%s\\n' 'pogo-test-passphrase-v2' ;;\n*) printf '%s\\n' 'pogo-test-passphrase-nope' ;;\nesac\n"
	default:
		body += "printf '%s\\n' 'pogo-test-passphrase'\n"
	}
	path := filepath.Join(dir, "ask.sh")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func promptSetup(t *testing.T) string {
	t.Helper()
	clearPromptEnv(t)
	t.Cleanup(passphrase.SetAskpassDirsForTest(nil))
	closedStdin(t)
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+"/usr/bin:/bin")
	return dir
}

func TestCreatePromptsAskpass(t *testing.T) {
	for _, cmd := range []string{"init", "encrypt", "convert"} {
		t.Run(cmd+" mismatch", func(t *testing.T) {
			dir := promptSetup(t)
			log := filepath.Join(dir, "log")
			t.Setenv("POGO_ASKPASS", writeAskpass(t, dir, log, "mismatch"))
			enc := filepath.Join(dir, "app.enc")
			var code int
			var stderr string
			if cmd == "init" {
				code, _, stderr = runCLI(t, "init", "-f", enc)
			} else {
				plain := filepath.Join(dir, ".env")
				if err := os.WriteFile(plain, []byte("A=1\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				code, _, stderr = runCLI(t, cmd, plain, "-f", enc)
			}
			if code != 4 || !strings.Contains(stderr, "did not match") {
				t.Fatalf("%d %s", code, stderr)
			}
			if _, err := os.Stat(enc); !os.IsNotExist(err) {
				t.Fatal("encrypted file was written")
			}
			text := readLog(t, log)
			if !strings.Contains(text, "New passphrase for app.enc:") || !strings.Contains(text, "Confirm new passphrase for app.enc:") {
				t.Fatalf("prompts %q", text)
			}
		})
		t.Run(cmd+" cancel second", func(t *testing.T) {
			dir := promptSetup(t)
			log := filepath.Join(dir, "log")
			t.Setenv("POGO_ASKPASS", writeAskpass(t, dir, log, "cancel2"))
			enc := filepath.Join(dir, "app.enc")
			var code int
			var stderr string
			if cmd == "init" {
				code, _, stderr = runCLI(t, "init", "-f", enc)
			} else {
				plain := filepath.Join(dir, ".env")
				if err := os.WriteFile(plain, []byte("A=1\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				code, _, stderr = runCLI(t, cmd, plain, "-f", enc)
			}
			if code != 1 || !strings.Contains(stderr, "cancelled") {
				t.Fatalf("%d %s", code, stderr)
			}
			if _, err := os.Stat(enc); !os.IsNotExist(err) {
				t.Fatal("encrypted file was written")
			}
		})
		t.Run(cmd+" no backend", func(t *testing.T) {
			dir := promptSetup(t)
			enc := filepath.Join(dir, "app.enc")
			var code int
			var stderr string
			if cmd == "init" {
				code, _, stderr = runCLI(t, "init", "-f", enc)
			} else {
				plain := filepath.Join(dir, ".env")
				if err := os.WriteFile(plain, []byte("A=1\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				code, _, stderr = runCLI(t, cmd, plain, "-f", enc)
			}
			if code != 1 || !strings.Contains(stderr, "pogo: no way to ask for the passphrase here") {
				t.Fatalf("%d %s", code, stderr)
			}
			if _, err := os.Stat(enc); !os.IsNotExist(err) {
				t.Fatal("encrypted file was written")
			}
		})
	}
}

func TestShortPassphraseStopsBeforeConfirm(t *testing.T) {
	dir := promptSetup(t)
	log := filepath.Join(dir, "log")
	t.Setenv("POGO_ASKPASS", writeAskpass(t, dir, log, "short"))
	enc := filepath.Join(dir, "e.enc")
	code, _, stderr := runCLI(t, "init", "-f", enc)
	if code != 4 || !strings.Contains(stderr, "too short") {
		t.Fatalf("%d %s", code, stderr)
	}
	text := readLog(t, log)
	if !strings.Contains(text, "New passphrase for e.enc:") || strings.Contains(text, "Confirm") {
		t.Fatalf("prompts %q", text)
	}
	if _, err := os.Stat(enc); !os.IsNotExist(err) {
		t.Fatal("encrypted file was written")
	}
}

func TestTimeoutKeepsSavedEdit(t *testing.T) {
	closedStdin(t)
	t.Setenv("POGO_EDIT_TIMEOUT", "1")
	dir := t.TempDir()
	marker := filepath.Join(dir, "where")
	script := filepath.Join(dir, "ed.sh")
	body := "#!/bin/sh\nprintf '%s\\n' \"$1\" > " + shQuote(marker) + "\nprintf '\\nB=2\\n' >> \"$1\"\nsleep 30\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errOut, done := capture(t)
	edited, code, err := editUntilValid([]byte("A=1\n"), editfile.Choice{Argv: []string{script}, Announce: true}, "e.enc", true)
	done()
	if err != nil || code != 0 || string(edited) != "A=1\n\nB=2\n" {
		t.Fatalf("%d %v %q\n%s", code, err, edited, errOut.String())
	}
	text := errOut.String()
	for _, part := range []string{"Pogo opened e.enc (decrypted)", "pogo waits up to 1 second; Ctrl+C to stop", "saved text was kept"} {
		if !strings.Contains(text, part) {
			t.Fatalf("missing %q\n%s", part, text)
		}
	}
	where, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(strings.TrimSpace(string(where))); !os.IsNotExist(err) {
		t.Fatalf("temp remains: %v", err)
	}
}

func TestPlainLabelKeepsEncName(t *testing.T) {
	if plainLabel("e.enc") != "e.enc" || plainLabel("/tmp/.env.enc") != ".env.enc" || plainLabel(".enc") != ".enc" {
		t.Fatalf("%q %q %q", plainLabel("e.enc"), plainLabel("/tmp/.env.enc"), plainLabel(".enc"))
	}
}

func TestOpenPromptsAskpass(t *testing.T) {
	for _, spec := range []struct {
		name string
		args func(enc string) []string
	}{
		{"run", func(enc string) []string { return []string{"run", "-f", enc, "--", "true"} }},
		{"edit", func(enc string) []string { return []string{"edit", "-f", enc} }},
		{"decrypt", func(enc string) []string { return []string{"decrypt", "-f", enc} }},
	} {
		t.Run(spec.name+" one prompt", func(t *testing.T) {
			dir := promptSetup(t)
			log := filepath.Join(dir, "log")
			t.Setenv("POGO_ASKPASS", writeAskpass(t, dir, log, "match"))
			if spec.name == "edit" {
				t.Setenv("POGO_EDITOR", "/bin/true")
			}
			enc := filepath.Join(dir, "app.enc")
			if err := os.WriteFile(enc, []byte("not-a-pogo-file"), 0o600); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(enc)
			code, stdout, stderr := runCLI(t, spec.args(enc)...)
			if code != 2 {
				t.Fatalf("%d %s %s", code, stdout, stderr)
			}
			if strings.Contains(stdout, "not-a-pogo-file") || strings.Contains(stderr, "not-a-pogo-file") {
				t.Fatal("plaintext-looking bytes were printed")
			}
			after, _ := os.ReadFile(enc)
			if string(after) != string(before) {
				t.Fatal("file changed")
			}
			text := readLog(t, log)
			if strings.Count(text, "\n") != 1 || !strings.Contains(text, "Enter passphrase for app.enc:") {
				t.Fatalf("prompts %q", text)
			}
		})
		t.Run(spec.name+" cancel", func(t *testing.T) {
			dir := promptSetup(t)
			log := filepath.Join(dir, "log")
			t.Setenv("POGO_ASKPASS", writeAskpass(t, dir, log, "cancel"))
			if spec.name == "edit" {
				t.Setenv("POGO_EDITOR", "/bin/true")
			}
			enc := filepath.Join(dir, "app.enc")
			if err := os.WriteFile(enc, []byte("not-a-pogo-file"), 0o600); err != nil {
				t.Fatal(err)
			}
			code, _, stderr := runCLI(t, spec.args(enc)...)
			if code != 1 || !strings.Contains(stderr, "cancelled") {
				t.Fatalf("%d %s", code, stderr)
			}
		})
		t.Run(spec.name+" no backend", func(t *testing.T) {
			dir := promptSetup(t)
			if spec.name == "edit" {
				t.Setenv("POGO_EDITOR", "/bin/true")
			}
			enc := filepath.Join(dir, "app.enc")
			if err := os.WriteFile(enc, []byte("not-a-pogo-file"), 0o600); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(enc)
			code, _, stderr := runCLI(t, spec.args(enc)...)
			if code != 1 || !strings.Contains(stderr, "pogo: no way to ask for the passphrase here") {
				t.Fatalf("%d %s", code, stderr)
			}
			after, _ := os.ReadFile(enc)
			if string(after) != string(before) {
				t.Fatal("file changed")
			}
		})
	}
}

func TestRekeyCurrentPromptOnJunk(t *testing.T) {
	dir := promptSetup(t)
	log := filepath.Join(dir, "log")
	t.Setenv("POGO_ASKPASS", writeAskpass(t, dir, log, "sequence"))
	enc := filepath.Join(dir, "app.enc")
	if err := os.WriteFile(enc, []byte("not-a-pogo-file"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runCLI(t, "rekey", "-f", enc)
	if code != 2 {
		t.Fatalf("%d %s", code, stderr)
	}
	text := readLog(t, log)
	if !strings.Contains(text, "Current passphrase for app.enc:") || strings.Contains(text, "New passphrase") {
		t.Fatalf("prompts %q", text)
	}
}

func TestKeyEnvSkipsAskpass(t *testing.T) {
	dir := promptSetup(t)
	log := filepath.Join(dir, "log")
	t.Setenv("POGO_ASKPASS", writeAskpass(t, dir, log, "match"))
	t.Setenv("POGO_SHORT", "short")
	enc := filepath.Join(dir, "app.enc")
	code, _, stderr := runCLI(t, "init", "-f", enc, "--key-env", "POGO_SHORT")
	if code != 4 || !strings.Contains(stderr, "too short") {
		t.Fatalf("%d %s", code, stderr)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatalf("askpass ran: %v", err)
	}
	if _, err := os.Stat(enc); !os.IsNotExist(err) {
		t.Fatal("file written")
	}
}

func TestEditorHostsFromCLI(t *testing.T) {
	dir := promptSetup(t)
	hosts := servePassSocket(t, []string{"pogo-test-passphrase", "other-passphrase-value"})
	t.Setenv("VSCODE_GIT_IPC_HANDLE", hosts.path)
	enc := filepath.Join(dir, "app.enc")
	code, _, stderr := runCLI(t, "init", "-f", enc)
	if code != 4 || !strings.Contains(stderr, "did not match") {
		t.Fatalf("%d %s", code, stderr)
	}
	if _, err := os.Stat(enc); !os.IsNotExist(err) {
		t.Fatal("file written")
	}
	got := hosts.snapshot()
	if len(got) != 2 || !strings.Contains(got[0], "pogo-new-passphrase-for-app.enc.invalid") || !strings.Contains(got[1], "pogo-confirm-passphrase-for-app.enc.invalid") {
		t.Fatalf("hosts %v", got)
	}

	dir = promptSetup(t)
	hosts = servePassSocket(t, []string{"pogo-test-passphrase"})
	t.Setenv("VSCODE_GIT_IPC_HANDLE", hosts.path)
	t.Setenv("POGO_EDITOR", "/bin/true")
	enc = filepath.Join(dir, ".env.enc")
	if err := os.WriteFile(enc, []byte("not-a-pogo-file"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = runCLI(t, "edit", "-f", enc)
	if code != 2 {
		t.Fatalf("%d %s", code, stderr)
	}
	got = hosts.snapshot()
	if len(got) != 1 || !strings.Contains(got[0], "https://pogo-passphrase@pogo-passphrase-for-.env.enc.invalid") {
		t.Fatalf("open host %v", got)
	}
}

func TestEditNoEditorLeavesFile(t *testing.T) {
	dir := promptSetup(t)
	t.Setenv("EDITOR", "vim")
	t.Setenv("VISUAL", "nano")
	enc := filepath.Join(dir, ".env.enc")
	body := []byte("ciphertext-marker-do-not-print")
	if err := os.WriteFile(enc, body, 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runCLI(t, "edit", "-f", enc)
	if code != 1 {
		t.Fatalf("%d %s", code, stderr)
	}
	for _, part := range []string{editfile.NoEditorSummary, editfile.AgentEditLine, "POGO_EDITOR", "integrated terminal"} {
		if !strings.Contains(stderr, part) {
			t.Errorf("missing %q\n%s", part, stderr)
		}
	}
	if strings.Contains(stdout+stderr, "ciphertext-marker-do-not-print") {
		t.Fatal("decrypted-looking bytes were printed")
	}
	got, err := os.ReadFile(enc)
	if err != nil || string(got) != string(body) {
		t.Fatalf("file changed %q %v", got, err)
	}
}

func TestEditAnnounceAndValidation(t *testing.T) {
	closedStdin(t)
	dir := t.TempDir()
	script := filepath.Join(dir, "ed.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errOut, done := capture(t)
	choice := editfile.Choice{Argv: []string{script}, Source: "remote-cli", Announce: true}
	out, err := openInEditor([]byte("A=1\n"), choice, ".env.enc", true)
	done()
	if err != nil || string(out) != "A=1\n" {
		t.Fatalf("%v %q", err, out)
	}
	want := "Pogo opened .env.enc (decrypted) in the editor. Save and close the tab to continue (pogo waits up to 10 minutes; Ctrl+C to stop).\n"
	if errOut.String() != want {
		t.Fatalf("announce %q", errOut.String())
	}

	marker := filepath.Join(dir, "where")
	bad := filepath.Join(dir, "bad.sh")
	if err := os.WriteFile(bad, []byte("#!/bin/sh\nprintf '%s\\n' \"$1\" > "+shQuote(marker)+"\nprintf 'not valid\\n' > \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	choice = editfile.Choice{Argv: []string{bad}, Source: "EDITOR"}
	_, code, err := editUntilValid([]byte("A=1\n"), choice, ".env.enc", true)
	if code != 3 || err == nil || !strings.Contains(err.Error(), "not valid") {
		t.Fatalf("%d %v", code, err)
	}
	where, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(strings.TrimSpace(string(where))); !os.IsNotExist(err) {
		t.Fatalf("temp remains %v", err)
	}

	same := filepath.Join(dir, "same.sh")
	if err := os.WriteFile(same, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	edited, code, err := editUntilValid([]byte("A=1\n"), editfile.Choice{Argv: []string{same}}, ".env.enc", true)
	if err != nil || code != 0 || string(edited) != "A=1\n" {
		t.Fatalf("%d %v %q", code, err, edited)
	}
}

func TestEditRoundTripNoTTY(t *testing.T) {
	if testing.Short() {
		t.Skip("full Argon2id")
	}
	dir := promptSetup(t)
	t.Setenv("XDG_RUNTIME_DIR", "")
	enc := filepath.Join(dir, ".env.enc")
	pass := []byte("pogo-test-passphrase")
	blob, err := filefmt.Seal(pass, []byte("A=1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(enc, blob, 0o600); err != nil {
		t.Fatal(err)
	}
	original := append([]byte(nil), blob...)

	log := filepath.Join(dir, "log")
	t.Setenv("POGO_ASKPASS", writeAskpass(t, dir, log, "match"))
	same := filepath.Join(dir, "same.sh")
	if err := os.WriteFile(same, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", same)
	code, _, stderr := runCLI(t, "edit", "-f", enc)
	if code != 0 || !strings.Contains(stderr, "No changes") {
		t.Fatalf("unchanged %d %s", code, stderr)
	}
	got, err := os.ReadFile(enc)
	if err != nil || string(got) != string(original) {
		t.Fatal("unchanged edit rewrote the file")
	}

	os.Remove(log)
	cli := filepath.Join(dir, "cursor")
	if err := os.WriteFile(cli, []byte("#!/bin/sh\nfile=\nfor a in \"$@\"; do file=\"$a\"; done\nsleep 0.1\nprintf '\\nB=2\\n' >> \"$file\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+"/usr/bin:/bin")
	t.Setenv("EDITOR", "vim")
	t.Setenv("VSCODE_IPC_HOOK_CLI", filepath.Join(dir, "hook"))
	t.Setenv("TERM_PROGRAM", "cursor")
	code, _, stderr = runCLI(t, "edit", "-f", enc)
	want := "Pogo opened .env.enc (decrypted) in the editor. Save and close the tab to continue (pogo waits up to 10 minutes; Ctrl+C to stop)."
	if code != 0 || !strings.Contains(stderr, want) || !strings.Contains(stderr, "Updated") {
		t.Fatalf("remote %d %s", code, stderr)
	}
	changed, err := os.ReadFile(enc)
	if err != nil || string(changed) == string(original) {
		t.Fatal("remote edit did not rewrite")
	}

	os.Remove(log)
	t.Setenv("POGO_ASKPASS", writeAskpass(t, dir, log, "sequence"))
	t.Setenv("VSCODE_IPC_HOOK_CLI", "")
	before, _ := os.ReadFile(enc)
	code, _, stderr = runCLI(t, "rekey", "-f", enc)
	if code != 4 || !strings.Contains(stderr, "did not match") {
		t.Fatalf("rekey %d %s", code, stderr)
	}
	after, _ := os.ReadFile(enc)
	if string(after) != string(before) {
		t.Fatal("rekey mismatch wrote")
	}
	text := readLog(t, log)
	for _, part := range []string{"Current passphrase for .env.enc:", "New passphrase for .env.enc:", "Confirm new passphrase for .env.enc:"} {
		if !strings.Contains(text, part) {
			t.Errorf("missing %q in %q", part, text)
		}
	}
}

func TestUpgradeCommand(t *testing.T) {
	clearPromptEnv(t)
	closedStdin(t)
	code, stdout, stderr := runCLI(t, "upgrade", "--help")
	if code != 0 || !strings.Contains(stdout, "--check") || !strings.Contains(stdout, "10") || !strings.Contains(stdout, "passphrase") {
		t.Fatalf("help %d %s %s", code, stdout, stderr)
	}
	code, _, stderr = runCLI(t, "upgrade", "--key-file", "x")
	if code != 1 || !strings.Contains(stderr, "removed on purpose") {
		t.Fatalf("key-file %d %s", code, stderr)
	}
	code, _, stderr = runCLI(t, "upgrade", "--version")
	if code != 1 || !strings.Contains(stderr, "--version") {
		t.Fatalf("version value %d %s", code, stderr)
	}
	code, _, stderr = runCLI(t, "upgrade", "-V")
	if code != 1 || !strings.Contains(stderr, "pogo upgrade --version") {
		t.Fatalf("-V %d %s", code, stderr)
	}

	srv := http.NewServeMux()
	srv.HandleFunc("/repos/JacksonJRay/pogo/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("token sent to test server")
		}
		_, _ = io.WriteString(w, `{"tag_name":"v1.0.0-test","assets":[]}`)
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	httpSrv := &http.Server{Handler: srv}
	go httpSrv.Serve(ln)
	t.Cleanup(func() { httpSrv.Close() })
	t.Setenv("POGO_UPGRADE_API", "http://"+ln.Addr().String())
	t.Setenv("GITHUB_TOKEN", "should-not-leak")
	code, stdout, stderr = runCLI(t, "upgrade", "--check")
	if code != 0 || stdout != "pogo 1.0.0-test is the latest version\n" {
		t.Fatalf("latest %d %q %s", code, stdout, stderr)
	}
}

type passSocket struct {
	path  string
	mu    sync.Mutex
	hosts []string
}

func (p *passSocket) snapshot() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := append([]string(nil), p.hosts...)
	return out
}

func servePassSocket(t *testing.T, passwords []string) *passSocket {
	t.Helper()
	ps := &passSocket{path: filepath.Join(t.TempDir(), "git.sock")}
	ln, err := net.Listen("unix", ps.path)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		var payload struct {
			Argv []string `json:"argv"`
		}
		_ = json.Unmarshal(body, &payload)
		ps.mu.Lock()
		if len(payload.Argv) >= 5 {
			ps.hosts = append(ps.hosts, payload.Argv[4])
		}
		i := n
		if i >= len(passwords) {
			i = len(passwords) - 1
		}
		n++
		pass := passwords[i]
		ps.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pass + "\n")
	})}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return ps
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
