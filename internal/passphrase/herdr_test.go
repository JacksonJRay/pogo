package passphrase

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func noTerminal(t *testing.T) {
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

func TestHerdrAskAndDoctor(t *testing.T) {
	noTerminal(t)
	dir := t.TempDir()
	log := filepath.Join(dir, "herdr.log")
	script := filepath.Join(dir, "herdr")
	body := `#!/bin/sh
printf '%s\n' "$*" >> "$POGO_HERDR_FAKE_LOG"
case " $* " in
*" pane split "*)
  printf '%s\n' '{"result":{"pane":{"pane_id":"w9:p9"}}}'
  ;;
*" pane run "*)
  script=
  for arg in "$@"; do script=$arg; done
  sock=$(awk -F"'" '/POGO_HERDR_SOCKET/{print $2; exit}' "$script")
  cp "$script" "$POGO_HERDR_FAKE_DIR/ask.sh"
  python3 -c 'import socket,sys; s=socket.socket(socket.AF_UNIX); s.connect(sys.argv[1]); s.sendall(b"ok\nfrom-herdr-test\n"); s.close()' "$sock"
  ;;
*" pane close "*)
  ;;
*)
  echo "unexpected $*" >&2
  exit 2
  ;;
esac
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("HERDR_PANE_ID", "w2:p1")
	t.Setenv("POGO_HERDR_FAKE_LOG", log)
	t.Setenv("POGO_HERDR_FAKE_DIR", dir)
	t.Setenv("POGO_ASKPASS", "")
	t.Setenv("VSCODE_GIT_IPC_HANDLE", "")
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")

	got, err := Acquire(Source{Prompt: "Enter passphrase: ", File: ".env.enc"})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "from-herdr-test" {
		t.Fatalf("%q", string(got))
	}
	scriptText, err := os.ReadFile(filepath.Join(dir, "ask.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(scriptText), "Enter passphrase for key") || !strings.Contains(string(scriptText), ".env.enc") {
		t.Fatalf("prompt missing from pane script\n%s", scriptText)
	}
	if strings.Contains(string(scriptText), "from-herdr-test") {
		t.Fatal("passphrase was written into the pane script")
	}
	logged, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logged), "pane close w9:p9") {
		t.Fatalf("pane was not closed\n%s", logged)
	}

	d := Diagnose()
	if !d.Available || d.Backend != HerdrLabel {
		t.Fatalf("%+v", d)
	}
	if strings.Contains(d.Report(), "from-herdr-test") {
		t.Fatal("doctor printed the passphrase")
	}
}

func TestHerdrCancel(t *testing.T) {
	noTerminal(t)
	dir := t.TempDir()
	script := filepath.Join(dir, "herdr")
	body := `#!/bin/sh
case " $* " in
*" pane split "*)
  printf '%s\n' '{"result":{"pane":{"pane_id":"w9:p2"}}}'
  ;;
*" pane run "*)
  script=
  for arg in "$@"; do script=$arg; done
  sock=$(awk -F"'" '/POGO_HERDR_SOCKET/{print $2; exit}' "$script")
  python3 -c 'import socket,sys; s=socket.socket(socket.AF_UNIX); s.connect(sys.argv[1]); s.sendall(b"cancel\n"); s.close()' "$sock"
  ;;
esac
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("HERDR_PANE_ID", "w2:p1")
	t.Setenv("POGO_ASKPASS", "")
	t.Setenv("VSCODE_GIT_IPC_HANDLE", "")

	_, err := Acquire(Source{Prompt: "Enter passphrase: ", File: ".env.enc"})
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("%v", err)
	}
}

func TestHerdrTimeoutClosesPane(t *testing.T) {
	noTerminal(t)
	dir := t.TempDir()
	log := filepath.Join(dir, "herdr.log")
	script := filepath.Join(dir, "herdr")
	body := `#!/bin/sh
printf '%s\n' "$*" >> "$POGO_HERDR_FAKE_LOG"
case " $* " in
*" pane split "*)
  printf '%s\n' '{"result":{"pane":{"pane_id":"w9:p3"}}}'
  ;;
*" pane run "*)
  sleep 30
  ;;
esac
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("HERDR_PANE_ID", "w2:p1")
	t.Setenv("POGO_HERDR_FAKE_LOG", log)
	t.Setenv("POGO_ASKPASS", "")
	t.Setenv("VSCODE_GIT_IPC_HANDLE", "")
	t.Cleanup(SetPromptTimeoutsForTest(50*time.Millisecond, 200*time.Millisecond))

	_, err := Acquire(Source{Prompt: "Enter passphrase: ", File: ".env.enc"})
	if err == nil || !strings.Contains(err.Error(), "Herdr prompt got no answer") {
		t.Fatalf("%v", err)
	}
	logged, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logged), "pane close w9:p3") {
		t.Fatalf("pane was not closed\n%s", logged)
	}
}
