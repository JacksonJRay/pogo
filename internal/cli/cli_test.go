package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"pogo/internal/dotenv"
	"pogo/internal/filefmt"
	"pogo/internal/skill"
)

func capture(t *testing.T) (out, errOut *bytes.Buffer, done func()) {
	t.Helper()
	or, ow, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	er, ew, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = ow, ew
	ob, eb := &bytes.Buffer{}, &bytes.Buffer{}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); io.Copy(ob, or) }()
	go func() { defer wg.Done(); io.Copy(eb, er) }()
	var once sync.Once
	done = func() {
		once.Do(func() {
			_ = ow.Close()
			_ = ew.Close()
			os.Stdout, os.Stderr = oldOut, oldErr
			wg.Wait()
			_ = or.Close()
			_ = er.Close()
		})
	}
	t.Cleanup(done)
	return ob, eb, done
}

func runCLI(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	out, errOut, done := capture(t)
	code = Main(args, "1.0.0-test")
	done()
	return code, out.String(), errOut.String()
}

func TestHelpFormsMatch(t *testing.T) {
	for _, cmd := range commandNames {
		longCode, longOut, longErr := runCLI(t, cmd, "--help")
		shortCode, shortOut, _ := runCLI(t, cmd, "-h")
		helpCode, helpOut, helpErr := runCLI(t, "help", cmd)
		if longCode != 0 || shortCode != 0 || helpCode != 0 {
			t.Fatalf("%s help codes %d %d %d stderr %s %s", cmd, longCode, shortCode, helpCode, longErr, helpErr)
		}
		if longOut != shortOut || longOut != helpOut {
			t.Fatalf("%s help text mismatch\n--help:\n%s\n-h:\n%s\nhelp:\n%s", cmd, longOut, shortOut, helpOut)
		}
		for _, heading := range []string{"Purpose\n", "Synopsis\n", "Arguments\n", "Flags\n", "Examples\n", "Common workflows\n", "Framework notes\n", "Security notes\n", "Exit codes\n"} {
			if !strings.Contains(longOut, heading) {
				t.Errorf("%s help missing %q", cmd, heading)
			}
		}
		if strings.Count(longOut, "\n    1. ") != 1 || !strings.Contains(longOut, "\n    2. ") || !strings.Contains(longOut, "\n    3. ") {
			t.Errorf("%s help should have at least 3 examples\n%s", cmd, longOut)
		}
		if !strings.Contains(longOut, "passphrase") {
			t.Errorf("%s help should explain passphrase in plain language", cmd)
		}
		if cmd == "init" || cmd == "edit" {
			for _, phrase := range []string{"VISUAL", "code --wait", "vim"} {
				if !strings.Contains(longOut, phrase) {
					t.Errorf("%s help missing %q", cmd, phrase)
				}
			}
		}
		if cmd == "docker" {
			for _, phrase := range []string{"Enter passphrase (min 15 characters):", "docker inspect", "--key-stdin", "PID 1", "read -rs", "op read", "docker compose run -T", "-it", "Which method should I pick?"} {
				if !strings.Contains(longOut, phrase) {
					t.Errorf("%s help missing %q", cmd, phrase)
				}
			}
			runCode, runOut, _ := runCLI(t, "run", "--help")
			if runCode != 0 || !strings.Contains(runOut, "read -rs") || !strings.Contains(runOut, "docker inspect") {
				t.Fatal("pogo run --help is missing the docker passphrase recipes")
			}
			bareCode, bareOut, _ := runCLI(t, "docker")
			if bareCode != 0 || bareOut != longOut {
				t.Fatalf("pogo docker should print the same guide as --help (code %d)", bareCode)
			}
		}
	}
	c1, g1, _ := runCLI(t, "--help")
	c2, g2, _ := runCLI(t, "-h")
	c3, g3, _ := runCLI(t, "help")
	if c1 != 0 || c2 != 0 || c3 != 0 || g1 != g2 || g1 != g3 {
		t.Fatalf("global help mismatch %d %d %d", c1, c2, c3)
	}
	if !strings.Contains(g1, "Quick start") {
		t.Fatal("global help missing quick start")
	}
	for _, heading := range []string{"Purpose\n", "Synopsis\n", "Flags\n", "Examples\n", "Exit codes\n"} {
		if !strings.Contains(g1, heading) {
			t.Errorf("global help missing %q", heading)
		}
	}
}

func TestUnknownCommandAndFlag(t *testing.T) {
	code, _, stderr := runCLI(t, "rn")
	if code != 1 || !strings.Contains(stderr, "Did you mean") || !strings.Contains(stderr, "run") || !strings.Contains(stderr, "--help") {
		t.Fatalf("code %d stderr %s", code, stderr)
	}
	code, _, stderr = runCLI(t, "run", "--keyenv", "x", "--", "true")
	if code != 1 || !strings.Contains(stderr, "Did you mean") || !strings.Contains(stderr, "--key-env") {
		t.Fatalf("flag code %d stderr %s", code, stderr)
	}
	code, _, stderr = runCLI(t, "run", "--key-file", "x", "--", "true")
	if code != 1 || !strings.Contains(stderr, "removed on purpose") || !strings.Contains(stderr, "--key-stdin") {
		t.Fatalf("removed key-file %d %s", code, stderr)
	}
	code, _, stderr = runCLI(t, "rekey", "--new-key-file", "x")
	if code != 1 || !strings.Contains(stderr, "removed on purpose") || !strings.Contains(stderr, "--new-key-env") {
		t.Fatalf("removed new-key-file %d %s", code, stderr)
	}
	code, _, stderr = runCLI(t, "version", "--key-env", "x")
	if code != 1 || !strings.Contains(stderr, "not a flag") || !strings.Contains(stderr, "--help") {
		t.Fatalf("wrong command flag %d %s", code, stderr)
	}
}

func TestUsageAndShortPassphrase(t *testing.T) {
	code, _, stderr := runCLI(t)
	if code != 1 || !strings.Contains(stderr, "No command given") {
		t.Fatalf("no args %d %s", code, stderr)
	}
	code, _, stderr = runCLI(t, "run")
	if code != 1 || !strings.Contains(stderr, "--") {
		t.Fatalf("run usage %d %s", code, stderr)
	}
	code, _, stderr = runCLI(t, "run", "npm", "start")
	if code != 1 || !strings.Contains(stderr, "pogo run -- npm") {
		t.Fatalf("run missing dash %d %s", code, stderr)
	}
	code, _, stderr = runCLI(t, "encrypt")
	if code != 1 {
		t.Fatalf("encrypt usage %d %s", code, stderr)
	}
	code, _, stderr = runCLI(t, "convert")
	if code != 1 {
		t.Fatalf("convert usage %d %s", code, stderr)
	}
	dir := t.TempDir()
	t.Setenv("POGO_TEST_PASS", "short")
	code, _, stderr = runCLI(t, "init", "-f", filepath.Join(dir, ".env.enc"), "--key-env", "POGO_TEST_PASS")
	if code != 4 || !strings.Contains(stderr, "too short") {
		t.Fatalf("init short %d %s", code, stderr)
	}
	plain := filepath.Join(dir, ".env")
	if err := os.WriteFile(plain, []byte("A=b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = runCLI(t, "convert", plain, "-f", filepath.Join(dir, "out.enc"), "--key-env", "POGO_TEST_PASS")
	if code != 4 {
		t.Fatalf("convert short %d %s", code, stderr)
	}
	enc := filepath.Join(dir, "re.enc")
	if err := os.WriteFile(enc, []byte("not a real file"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = runCLI(t, "rekey", "-f", enc, "--key-env", "POGO_TEST_PASS")
	if code != 4 {
		t.Fatalf("rekey short %d %s", code, stderr)
	}
	code, _, stderr = runCLI(t, "init", "-f", filepath.Join(dir, "tty.enc"))
	if code != 1 || !strings.Contains(stderr, "not a terminal") || !strings.Contains(stderr, "-it") || !strings.Contains(stderr, "--key-stdin") {
		t.Fatalf("no tty %d %s", code, stderr)
	}
	code, _, stderr = runCLI(t, "init", "--min-length", "0")
	if code != 1 || !strings.Contains(stderr, "cannot be less than 1") {
		t.Fatalf("min 0 %d %s", code, stderr)
	}
	code, _, stderr = runCLI(t, "init", "-f", filepath.Join(dir, "a.enc"), "other.enc")
	if code != 1 || !strings.Contains(stderr, "two different") {
		t.Fatalf("conflict %d %s", code, stderr)
	}
	code, _, stderr = runCLI(t, "run", "--key-env", "X", "--key-stdin", "--", "true")
	if code != 1 || !strings.Contains(stderr, "only one") {
		t.Fatalf("multi key %d %s", code, stderr)
	}
}

func TestMissingFileAndBadMagic(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("POGO_TEST_PASS", "pogo-test-passphrase")
	missing := filepath.Join(dir, ".env.enc")
	code, _, stderr := runCLI(t, "run", "-f", missing, "--key-env", "POGO_TEST_PASS", "--", "true")
	if code != 1 || !strings.Contains(stderr, "No ") || !strings.Contains(stderr, "pogo init") || !strings.Contains(stderr, "pogo convert") {
		t.Fatalf("missing %d %s", code, stderr)
	}
	bad := filepath.Join(dir, "bad.enc")
	if err := os.WriteFile(bad, []byte("this is not pogo ciphertext!!!!"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = runCLI(t, "decrypt", "-f", bad, "--key-env", "POGO_TEST_PASS")
	if code != 2 || !strings.Contains(stderr, "not a usable Pogo") {
		t.Fatalf("bad magic %d %s", code, stderr)
	}
}

func TestSkillAndVersion(t *testing.T) {
	code, stdout, stderr := runCLI(t, "skill")
	if code != 0 {
		t.Fatal(stderr)
	}
	if stdout != string(skill.Document) {
		t.Fatalf("skill stdout mismatch %d vs %d", len(stdout), len(skill.Document))
	}
	code, stdout, _ = runCLI(t, "version")
	if code != 0 || stdout != "pogo 1.0.0-test\n" {
		t.Fatalf("version %d %q", code, stdout)
	}
	code, stdout, _ = runCLI(t, "--version")
	if code != 0 || !strings.Contains(stdout, "1.0.0-test") {
		t.Fatalf("--version %d %q", code, stdout)
	}
	code, stdout, stderr = runCLI(t, "skill", "--list")
	if code != 0 || !strings.Contains(stdout, "express") || !strings.Contains(stdout, "sveltekit") {
		t.Fatalf("list %d %s %s", code, stdout, stderr)
	}
	code, _, stderr = runCLI(t, "skill", "--framework", "expresss")
	if code != 1 || !strings.Contains(stderr, "Did you mean") {
		t.Fatalf("framework suggest %d %s", code, stderr)
	}
}

func TestBuildEnvOverride(t *testing.T) {
	base := []string{"FOO=old", "FOO=older", "KEEP=1", "POGO_PASSPHRASE=secret"}
	got := buildEnv(base, []dotenv.Pair{{Key: "FOO", Value: "from-file"}, {Key: "NEW", Value: "n"}}, map[string]struct{}{"POGO_PASSPHRASE": {}}, true)
	if !containsEnv(got, "FOO", "from-file") || !containsEnv(got, "NEW", "n") || !containsEnv(got, "KEEP", "1") {
		t.Fatalf("%v", got)
	}
	if containsKey(got, "POGO_PASSPHRASE") {
		t.Fatal("passphrase leaked")
	}
	got = buildEnv(base, []dotenv.Pair{{Key: "FOO", Value: "from-file"}}, nil, false)
	if !containsEnv(got, "FOO", "old") {
		t.Fatalf("no-override got %v", got)
	}
}

func containsEnv(env []string, k, v string) bool {
	for _, e := range env {
		if e == k+"="+v {
			return true
		}
	}
	return false
}

func containsKey(env []string, k string) bool {
	for _, e := range env {
		if strings.HasPrefix(e, k+"=") {
			return true
		}
	}
	return false
}

func TestSlowExitCodes(t *testing.T) {
	if testing.Short() {
		t.Skip("full Argon2id")
	}
	dir := t.TempDir()
	plain := filepath.Join(dir, ".env")
	enc := filepath.Join(dir, ".env.enc")
	t.Setenv("POGO_TEST_PASS", "pogo-test-passphrase")
	t.Setenv("POGO_TEST_PASS_WRONG", "pogo-test-passphrase-nope")
	t.Setenv("POGO_TEST_PASS_NEW", "pogo-test-passphrase-v2")
	t.Setenv("POGO_TEST_PASS_SHORT", "too-short")
	body := "API_KEY=pogo_test_api_key_example\nQUOTED_VALUE=\"say \\\"hello\\\" world\"\n"
	if err := os.WriteFile(plain, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runCLI(t, "convert", plain, "-f", enc, "--key-env", "POGO_TEST_PASS")
	if code != 0 {
		t.Fatalf("convert %d %s", code, stderr)
	}
	if !strings.Contains(stderr, "Left the plaintext") || !strings.Contains(stderr, "delete") {
		t.Fatalf("reminder missing: %s", stderr)
	}
	after, err := os.ReadFile(plain)
	if err != nil || string(after) != body {
		t.Fatal("plaintext was modified")
	}
	blob, err := os.ReadFile(enc)
	if err != nil {
		t.Fatal(err)
	}
	h, _, err := filefmt.Split(blob)
	if err != nil {
		t.Fatal(err)
	}
	if h.Params != filefmt.DefaultParams {
		t.Fatalf("header %+v", h.Params)
	}
	code, stdout, stderr := runCLI(t, "decrypt", "-f", enc, "--key-env", "POGO_TEST_PASS")
	if code != 0 || stdout != body {
		t.Fatalf("decrypt %d %q %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "Warning:") {
		t.Fatalf("warning missing %s", stderr)
	}
	code, _, stderr = runCLI(t, "decrypt", "-f", enc, "--key-env", "POGO_TEST_PASS_WRONG")
	if code != 2 {
		t.Fatalf("wrong key %d %s", code, stderr)
	}
	tampered := append([]byte(nil), blob...)
	tampered[len(tampered)-1] ^= 0x5a
	tamperPath := filepath.Join(dir, "tamper.enc")
	if err := os.WriteFile(tamperPath, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, _ = runCLI(t, "decrypt", "-f", tamperPath, "--key-env", "POGO_TEST_PASS")
	if code != 2 {
		t.Fatalf("tamper %d", code)
	}
	t.Setenv("EDITOR", "false")
	code, _, stderr = runCLI(t, "edit", "-f", enc, "--key-env", "POGO_TEST_PASS")
	if code != 3 {
		t.Fatalf("editor %d %s", code, stderr)
	}
	code, _, _ = runCLI(t, "rekey", "-f", enc, "--key-env", "POGO_TEST_PASS", "--new-key-env", "POGO_TEST_PASS_SHORT")
	if code != 4 {
		t.Fatalf("rekey short new %d", code)
	}
	code, _, stderr = runCLI(t, "rekey", "-f", enc, "--key-env", "POGO_TEST_PASS", "--new-key-env", "POGO_TEST_PASS_NEW")
	if code != 0 {
		t.Fatalf("rekey %d %s", code, stderr)
	}
	code, _, _ = runCLI(t, "decrypt", "-f", enc, "--key-env", "POGO_TEST_PASS")
	if code != 2 {
		t.Fatalf("old key after rekey %d", code)
	}
	code, stdout, stderr = runCLI(t, "decrypt", "-f", enc, "--key-env", "POGO_TEST_PASS_NEW")
	if code != 0 || stdout != body {
		t.Fatalf("new key decrypt %d %q %s", code, stdout, stderr)
	}
	code, _, stderr = runCLI(t, "run", "-f", enc, "--key-env", "POGO_TEST_PASS_NEW", "--", "pogo-this-command-does-not-exist")
	if code != 1 || !strings.Contains(stderr, "Could not find command") {
		t.Fatalf("missing cmd %d %s", code, stderr)
	}
}

func TestEnvHelpersCompileUse(t *testing.T) {
	doc, err := dotenv.Parse([]byte("B=2\n"))
	if err != nil {
		t.Fatal(err)
	}
	pairs, err := doc.Select("")
	if err != nil {
		t.Fatal(err)
	}
	got := buildEnv([]string{"A=1"}, pairs, nil, true)
	if !containsEnv(got, "A", "1") || !containsEnv(got, "B", "2") {
		t.Fatalf("%v", got)
	}
}
