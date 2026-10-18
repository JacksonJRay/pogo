package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	// A developer running go test from a terminal has a controlling terminal.
	// Convert must not block those tests on the real /dev/tty. Tests that
	// want an answer install their own reader.
	deleteAnswerReader = func() (io.ReadWriteCloser, error) {
		return nil, errors.New("no controlling terminal")
	}
	os.Exit(m.Run())
}

type scriptTTY struct {
	r      *bytes.Reader
	prompt bytes.Buffer
}

func (s *scriptTTY) Read(p []byte) (int, error)  { return s.r.Read(p) }
func (s *scriptTTY) Write(p []byte) (int, error) { return s.prompt.Write(p) }
func (s *scriptTTY) Close() error                { return nil }

func useDeleteAnswer(t *testing.T, answer string) *scriptTTY {
	t.Helper()
	tty := &scriptTTY{r: bytes.NewReader([]byte(answer))}
	prev := deleteAnswerReader
	deleteAnswerReader = func() (io.ReadWriteCloser, error) {
		return tty, nil
	}
	t.Cleanup(func() { deleteAnswerReader = prev })
	return tty
}

func TestAnsweredYes(t *testing.T) {
	yes := []string{"y", "Y", "yes", "YES", "Yes", "  y  ", "yes\r", " y\n"}
	no := []string{"", "n", "N", "no", "No", "yeah", "yep", " ", "\n", "true", "1"}
	for _, line := range yes {
		if !answeredYes(line) {
			t.Errorf("answeredYes(%q) = false", line)
		}
	}
	for _, line := range no {
		if answeredYes(line) {
			t.Errorf("answeredYes(%q) = true", line)
		}
	}
}

func TestAskDeleteOriginal(t *testing.T) {
	t.Run("no terminal", func(t *testing.T) {
		yes, asked := askDeleteOriginal(".env")
		if yes || asked {
			t.Fatalf("yes %v asked %v", yes, asked)
		}
	})
	cases := []struct {
		name   string
		answer string
		want   bool
	}{
		{"y", "y\n", true},
		{"yes", "yes\n", true},
		{"YES", "YES\n", true},
		{"padded", "  Yes  \n", true},
		{"n", "n\n", false},
		{"no", "no\n", false},
		{"enter", "\n", false},
		{"other", "yeah\n", false},
		{"eof", "", false},
		{"partial without newline", "y", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tty := useDeleteAnswer(t, tc.answer)
			yes, asked := askDeleteOriginal(".env")
			if !asked {
				t.Fatal("did not ask")
			}
			if yes != tc.want {
				t.Fatalf("yes %v want %v", yes, tc.want)
			}
			if tty.prompt.String() != "Delete the original .env? [y/N] " {
				t.Fatalf("prompt %q", tty.prompt.String())
			}
		})
	}
	t.Run("quoted path", func(t *testing.T) {
		tty := useDeleteAnswer(t, "n\n")
		_, asked := askDeleteOriginal("my env")
		if !asked {
			t.Fatal("did not ask")
		}
		if tty.prompt.String() != "Delete the original 'my env'? [y/N] " {
			t.Fatalf("prompt %q", tty.prompt.String())
		}
	})
	t.Run("prompt write fails", func(t *testing.T) {
		prev := deleteAnswerReader
		deleteAnswerReader = func() (io.ReadWriteCloser, error) {
			return errTTY{}, nil
		}
		t.Cleanup(func() { deleteAnswerReader = prev })
		yes, asked := askDeleteOriginal(".env")
		if yes || asked {
			t.Fatalf("yes %v asked %v", yes, asked)
		}
	})
}

type errTTY struct{}

func (errTTY) Read([]byte) (int, error)  { return 0, errors.New("read") }
func (errTTY) Write([]byte) (int, error) { return 0, errors.New("closed") }
func (errTTY) Close() error              { return nil }

func TestConvertDeleteFlags(t *testing.T) {
	code, _, stderr := runCLI(t, "encrypt", "--delete-original")
	if code != 1 || !strings.Contains(stderr, "not a flag for encrypt") {
		t.Fatalf("encrypt flag %d %s", code, stderr)
	}
	code, _, stderr = runCLI(t, "convert", "plain.env", "--delete-original", "--keep-original")
	if code != 1 || !strings.Contains(stderr, "--delete-original") || !strings.Contains(stderr, "--keep-original") {
		t.Fatalf("both flags %d %s", code, stderr)
	}
	_, stdout, _ := runCLI(t, "convert", "--help")
	for _, phrase := range []string{
		"--delete-original",
		"--keep-original",
		"[y/N]",
		"not a secure wipe",
		"controlling terminal",
		"--key-stdin",
	} {
		if !strings.Contains(stdout, phrase) {
			t.Errorf("convert help missing %q", phrase)
		}
	}
	_, stdout, _ = runCLI(t, "--help")
	if !strings.Contains(stdout, "default is to keep") {
		t.Fatalf("quick start missing the default: %s", stdout)
	}
}

func TestConvertFailureKeepsPlaintext(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, ".env")
	enc := filepath.Join(dir, ".env.enc")
	body := "API_KEY=pogo_test_api_key_example\n"
	if err := os.WriteFile(plain, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("POGO_SHORT", "short")
	code, _, stderr := runCLI(t, "convert", plain, "-f", enc, "--key-env", "POGO_SHORT", "--delete-original")
	if code != 4 || !strings.Contains(stderr, "too short") {
		t.Fatalf("short %d %s", code, stderr)
	}
	got, err := os.ReadFile(plain)
	if err != nil || string(got) != body {
		t.Fatalf("plaintext changed: %v %q", err, got)
	}
	if _, err := os.Stat(enc); !os.IsNotExist(err) {
		t.Fatalf("encrypted file exists: %v", err)
	}
}

func TestVerifyRoundTripRejectsGarbage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.enc")
	if err := os.WriteFile(path, []byte("not a pogo file"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := verifyRoundTrip(path, []byte("pogo-test-passphrase"), []byte("A=b\n"))
	if err == nil || !strings.Contains(err.Error(), "left in place") {
		t.Fatalf("err %v", err)
	}
}

func TestConvertDeleteBehaviors(t *testing.T) {
	if testing.Short() {
		t.Skip("full Argon2id")
	}
	pass := "pogo-test-passphrase"
	t.Setenv("POGO_TEST_PASS", pass)
	body := "API_KEY=pogo_test_api_key_example\n"

	type outcome struct {
		name    string
		args    []string
		stdin   string
		answer  *string
		deleted bool
		noAsk   bool
		stderr  []string
		absent  []string
		code    int
	}
	y := "y\n"
	yes := "yes\n"
	upper := "YES\n"
	no := "n\n"
	enter := "\n"
	partial := "y"
	cases := []outcome{
		{name: "y", answer: &y, deleted: true, stderr: []string{"Removed ", "not a secure wipe", "Checked that"}, absent: []string{"Left the plaintext"}},
		{name: "yes", answer: &yes, deleted: true, stderr: []string{"Removed ", "not a secure wipe"}},
		{name: "YES", answer: &upper, deleted: true, stderr: []string{"Removed "}},
		{name: "n", answer: &no, stderr: []string{"Left the plaintext", "not a secure wipe"}, absent: []string{"No terminal", "Removed "}},
		{name: "enter", answer: &enter, stderr: []string{"Left the plaintext"}, absent: []string{"No terminal", "Removed "}},
		{name: "eof", answer: new(string), stderr: []string{"Left the plaintext"}, absent: []string{"Removed "}},
		{name: "partial y is eof", answer: &partial, stderr: []string{"Left the plaintext"}, absent: []string{"Removed "}},
		{name: "no tty", stderr: []string{"No terminal", "Left the plaintext", "--delete-original", "not a secure wipe"}, absent: []string{"Removed "}},
		{name: "flag delete", args: []string{"--delete-original"}, deleted: true, noAsk: true, stderr: []string{"Removed ", "not a secure wipe"}, absent: []string{"Left the plaintext"}},
		{name: "flag keep", args: []string{"--keep-original"}, answer: &y, noAsk: true, stderr: []string{"Left the plaintext"}, absent: []string{"No terminal", "Removed "}},
		{name: "key stdin y", args: []string{"--key-stdin"}, stdin: pass, answer: &y, deleted: true, stderr: []string{"Removed ", "Checked that"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			plain := filepath.Join(dir, ".env")
			enc := filepath.Join(dir, ".env.enc")
			if err := os.WriteFile(plain, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			var tty *scriptTTY
			if tc.answer != nil {
				tty = useDeleteAnswer(t, *tc.answer)
			}
			args := []string{"convert", plain, "-f", enc}
			if tc.stdin != "" {
				args = append(args, tc.args...)
			} else {
				args = append(args, "--key-env", "POGO_TEST_PASS")
				args = append(args, tc.args...)
			}
			var code int
			var stderr string
			if tc.stdin != "" {
				code, _, stderr = runCLIStdin(t, tc.stdin, args...)
			} else {
				code, _, stderr = runCLI(t, args...)
			}
			if code != tc.code {
				t.Fatalf("exit %d stderr %s", code, stderr)
			}
			_, statErr := os.Stat(plain)
			if tc.deleted {
				if !os.IsNotExist(statErr) {
					t.Fatalf("plaintext still there: %v", statErr)
				}
			} else if statErr != nil {
				t.Fatalf("plaintext: %v", statErr)
			} else {
				got, err := os.ReadFile(plain)
				if err != nil || string(got) != body {
					t.Fatalf("plaintext changed: %v %q", err, got)
				}
			}
			blob, err := os.ReadFile(enc)
			if err != nil {
				t.Fatal(err)
			}
			if len(blob) < 4 || string(blob[:4]) != "POGO" {
				t.Fatalf("bad ciphertext header %q", blob[:min(4, len(blob))])
			}
			for _, phrase := range tc.stderr {
				if !strings.Contains(stderr, phrase) {
					t.Errorf("stderr missing %q\n%s", phrase, stderr)
				}
			}
			for _, phrase := range tc.absent {
				if strings.Contains(stderr, phrase) {
					t.Errorf("stderr unexpectedly has %q\n%s", phrase, stderr)
				}
			}
			if tc.noAsk && tty != nil && tty.prompt.Len() != 0 {
				t.Fatalf("asked despite the flag: %q", tty.prompt.String())
			}
			if tc.answer != nil && !tc.noAsk && tty.prompt.String() != deletePrompt(plain) {
				t.Fatalf("prompt %q", tty.prompt.String())
			}
			if tc.name == "y" {
				code, stdout, stderr := runCLI(t, "decrypt", "-f", enc, "--key-env", "POGO_TEST_PASS")
				if code != 0 || stdout != body {
					t.Fatalf("decrypt %d %q %s", code, stdout, stderr)
				}
			}
		})
	}

	t.Run("verify failure keeps plaintext", func(t *testing.T) {
		dir := t.TempDir()
		plain := filepath.Join(dir, ".env")
		enc := filepath.Join(dir, ".env.enc")
		if err := os.WriteFile(plain, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		prev := verifySealed
		verifySealed = func(string, []byte, []byte) error {
			return errors.New("check failed")
		}
		t.Cleanup(func() { verifySealed = prev })
		code, _, stderr := runCLI(t, "convert", plain, "-f", enc, "--key-env", "POGO_TEST_PASS", "--delete-original")
		if code == 0 || !strings.Contains(stderr, "check failed") {
			t.Fatalf("exit %d %s", code, stderr)
		}
		got, err := os.ReadFile(plain)
		if err != nil || string(got) != body {
			t.Fatalf("plaintext: %v %q", err, got)
		}
		if _, err := os.Stat(enc); err != nil {
			t.Fatalf("encrypted file: %v", err)
		}
		if strings.Contains(stderr, "Removed ") {
			t.Fatalf("removed after a failed check: %s", stderr)
		}
	})
}
