package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConvertOutputPaths(t *testing.T) {
	plain, enc, err := encryptOutputPaths("", false, []string{".env-global"}, ".env.global")
	if err != nil || plain != ".env-global" || enc != ".env.global" {
		t.Fatalf("positional -o got %q %q %v", plain, enc, err)
	}
	plain, enc, err = encryptOutputPaths(".env-global", true, nil, ".env.global")
	if err != nil || plain != ".env-global" || enc != ".env.global" {
		t.Fatalf("-f plain -o enc got %q %q %v", plain, enc, err)
	}
	_, _, err = encryptOutputPaths("other.enc", true, []string{".env"}, ".env.global")
	if err == nil || !strings.Contains(err.Error(), "two encrypted output paths") {
		t.Fatalf("two outputs %v", err)
	}
	if testing.Short() {
		t.Skip("full Argon2id")
	}
	dir := t.TempDir()
	plain = filepath.Join(dir, ".env-global")
	enc = filepath.Join(dir, ".env.global")
	if err := os.WriteFile(plain, []byte("API_KEY=super-secret-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("POGO_TEST_PASS", "pogo-test-passphrase")
	code, stdout, stderr := runCLI(t, "convert", plain, "-o", enc, "--key-env", "POGO_TEST_PASS", "--keep-original")
	if code != 0 {
		t.Fatalf("convert -o %d %s", code, stderr)
	}
	if strings.Contains(stdout, "super-secret-value") || strings.Contains(stderr, "super-secret-value") {
		t.Fatalf("value leaked %s %s", stdout, stderr)
	}
	if _, err := os.Stat(enc); err != nil {
		t.Fatal(err)
	}
	enc2 := filepath.Join(dir, ".env.global.2")
	code, _, stderr = runCLI(t, "convert", "-f", plain, "-o", enc2, "--key-env", "POGO_TEST_PASS", "--keep-original")
	if code != 0 {
		t.Fatalf("convert -f plain -o enc %d %s", code, stderr)
	}
	if _, err := os.Stat(enc2); err != nil {
		t.Fatal(err)
	}
}

func TestDecryptPathsOutput(t *testing.T) {
	enc, out, toStdout, err := decryptPaths(".env.enc", true, nil, ".env", true)
	if err != nil || enc != ".env.enc" || out != ".env" || toStdout {
		t.Fatalf(" -o got %q %q stdout=%v %v", enc, out, toStdout, err)
	}
	enc, out, toStdout, err = decryptPaths("", false, nil, ".env", true)
	if err != nil || enc != ".env.enc" || out != ".env" || toStdout {
		t.Fatalf("default -o got %q %q stdout=%v %v", enc, out, toStdout, err)
	}
	enc, out, toStdout, err = decryptPaths(".env", true, []string{".env.enc"}, "", false)
	if err != nil || enc != ".env.enc" || out != ".env" || toStdout {
		t.Fatalf("positional enc -f out got %q %q stdout=%v %v", enc, out, toStdout, err)
	}
	enc, out, toStdout, err = decryptPaths(".env.enc", true, []string{"./check.env"}, "", false)
	if err != nil || enc != ".env.enc" || out != "./check.env" || toStdout {
		t.Fatalf("-f enc positional out got %q %q stdout=%v %v", enc, out, toStdout, err)
	}
	enc, out, toStdout, err = decryptPaths(".env.enc", true, nil, "", false)
	if err != nil || enc != ".env.enc" || out != "" || !toStdout {
		t.Fatalf("-f enc stdout got %q %q stdout=%v %v", enc, out, toStdout, err)
	}
	_, _, _, err = decryptPaths(".env.enc", true, nil, ".env.enc", true)
	if err == nil || !strings.Contains(err.Error(), "Refusing to write") {
		t.Fatalf("same path err %v", err)
	}
}

func TestKeysOmitsValuesAndDecryptWritesFile(t *testing.T) {
	if testing.Short() {
		t.Skip("full Argon2id")
	}
	dir := t.TempDir()
	plain := filepath.Join(dir, ".env")
	enc := filepath.Join(dir, ".env.enc")
	body := "API_KEY=super-secret-value\nPORT=43111\nAPI_KEY=later-secret\n\n[production]\nAPI_KEY=prod-secret\nSTRIPE_KEY=stripe-secret\n\n[dev]\nAPI_KEY=dev-secret\n"
	if err := os.WriteFile(plain, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("POGO_TEST_PASS", "pogo-test-passphrase")
	code, _, stderr := runCLI(t, "convert", plain, "-f", enc, "--key-env", "POGO_TEST_PASS", "--keep-original")
	if code != 0 {
		t.Fatalf("convert %d %s", code, stderr)
	}

	code, stdout, stderr := runCLI(t, "keys", "-f", enc, "--key-env", "POGO_TEST_PASS")
	if code != 0 {
		t.Fatalf("keys %d %s", code, stderr)
	}
	if strings.Contains(stdout, "super-secret-value") || strings.Contains(stdout, "prod-secret") || strings.Contains(stdout, "stripe-secret") || strings.Contains(stderr, "secret") {
		t.Fatalf("values leaked\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	if !strings.Contains(stderr, "Names only. Values were not printed.") {
		t.Fatalf("note missing %s", stderr)
	}
	want := "API_KEY\nPORT\n[production]\nAPI_KEY\nSTRIPE_KEY\n[dev]\nAPI_KEY\n"
	if stdout != want {
		t.Fatalf("names\n%s", stdout)
	}

	code, stdout, stderr = runCLI(t, "keys", "-f", enc, "--env", "production", "--key-env", "POGO_TEST_PASS")
	if code != 0 || stdout != "API_KEY\nPORT\nSTRIPE_KEY\n" {
		t.Fatalf("env keys %d %q %s", code, stdout, stderr)
	}
	if strings.Contains(stdout, "prod-secret") || strings.Contains(stdout, "super-secret") {
		t.Fatalf("env values leaked %s", stdout)
	}

	out := filepath.Join(dir, "check.env")
	code, stdout, stderr = runCLI(t, "decrypt", enc, "-f", out, "--key-env", "POGO_TEST_PASS")
	if code != 0 {
		t.Fatalf("decrypt -f out %d %s", code, stderr)
	}
	if stdout != "" {
		t.Fatalf("decrypt wrote stdout %q", stdout)
	}
	got, err := os.ReadFile(out)
	if err != nil || string(got) != body {
		t.Fatalf("file %q err %v", got, err)
	}
	st, err := os.Stat(out)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v %v", st, err)
	}
	code, _, stderr = runCLI(t, "decrypt", "-f", enc, "-o", out, "--key-env", "POGO_TEST_PASS")
	if code != 1 || !strings.Contains(stderr, "already exists") {
		t.Fatalf("overwrite %d %s", code, stderr)
	}
	forced := filepath.Join(dir, "forced.env")
	if err := os.WriteFile(forced, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = runCLI(t, "decrypt", "-f", enc, "-o", forced, "--force", "--key-env", "POGO_TEST_PASS")
	if code != 0 {
		t.Fatalf("force %d %s", code, stderr)
	}
	got, err = os.ReadFile(forced)
	if err != nil || string(got) != body {
		t.Fatalf("forced file %q err %v", got, err)
	}
}
