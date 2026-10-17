package upgrade

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestSemver(t *testing.T) {
	a, err := parseSemver("1.0.10")
	if err != nil {
		t.Fatal(err)
	}
	b, err := parseSemver("v1.0.9")
	if err != nil {
		t.Fatal(err)
	}
	if compare(a, b) != 1 {
		t.Fatal("1.0.10 should be newer than 1.0.9")
	}
	rel, err := parseSemver("1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	pre, err := parseSemver("1.0.0-alpha")
	if err != nil {
		t.Fatal(err)
	}
	if compare(pre, rel) != -1 {
		t.Fatal("prerelease should be older")
	}
	meta, err := parseSemver("v1.0.0+build.5")
	if err != nil {
		t.Fatal(err)
	}
	if compare(meta, rel) != 0 || meta.String() != "1.0.0" {
		t.Fatalf("build metadata %+v", meta)
	}
	if _, err := parseSemver("01.2.3"); err == nil {
		t.Fatal("leading zero accepted")
	}
	if _, err := parseSemver("1.2"); err == nil {
		t.Fatal("short version accepted")
	}
}

func TestManagedPaths(t *testing.T) {
	tool, ok := managedBy("/nix/store/abc/bin/pogo")
	if !ok || tool != "Nix" {
		t.Fatalf("%s %v", tool, ok)
	}
	tool, ok = managedBy("/gnu/store/abc/bin/pogo")
	if !ok || tool != "Guix" {
		t.Fatalf("%s %v", tool, ok)
	}
	if _, ok := managedBy("/usr/local/bin/pogo"); ok {
		t.Fatal("user path treated as managed")
	}
}

func TestUpgradeRefusesPlainHTTP(t *testing.T) {
	t.Setenv(apiEnv, "http://example.com")
	code, err := Run(Options{Current: "1.0.4", Check: true})
	if code != 1 || err == nil || !strings.Contains(err.Error(), "plain HTTP") {
		t.Fatalf("%d %v", code, err)
	}
}

func TestUpgradeFlows(t *testing.T) {
	asset := "pogo-" + runtime.GOOS + "-" + runtime.GOARCH
	oldHook, oldTerm := executablePath, stdinIsTerm
	t.Cleanup(func() {
		executablePath = oldHook
		stdinIsTerm = oldTerm
	})
	stdinIsTerm = func() bool { return false }

	srv := &fakeRelease{}
	httpSrv := httptest.NewServer(srv)
	t.Cleanup(httpSrv.Close)
	srv.base = httpSrv.URL
	t.Setenv(apiEnv, httpSrv.URL)
	t.Setenv("GITHUB_TOKEN", "should-not-leak")

	t.Run("up to date", func(t *testing.T) {
		srv.reset(t, "v1.0.4", map[string][]byte{})
		code, stdout, stderr := runUpgrade(t, Options{Current: "1.0.4"})
		if code != 0 || stdout != "pogo 1.0.4 is the latest version\n" || stderr != "" {
			t.Fatalf("code %d\n%s\n%s", code, stdout, stderr)
		}
		if srv.downloads != 0 || srv.sawAuth {
			t.Fatalf("downloads %d auth %v", srv.downloads, srv.sawAuth)
		}
	})

	t.Run("newer than latest", func(t *testing.T) {
		srv.reset(t, "v1.0.4", nil)
		code, stdout, _ := runUpgrade(t, Options{Current: "1.2.0"})
		if code != 0 || stdout != "pogo 1.2.0 is newer than the latest release 1.0.4\n" {
			t.Fatalf("%d %q", code, stdout)
		}
	})

	t.Run("check available", func(t *testing.T) {
		srv.reset(t, "v1.2.3", nil)
		code, stdout, _ := runUpgrade(t, Options{Current: "1.0.4", Check: true})
		if code != ExitUpgradeAvailable || stdout != "current 1.0.4\nlatest 1.2.3\n" {
			t.Fatalf("%d %q", code, stdout)
		}
		if srv.downloads != 0 {
			t.Fatal("check downloaded an asset")
		}
	})

	t.Run("check pin", func(t *testing.T) {
		srv.reset(t, "v1.0.0", nil)
		srv.setTag("v1.0.0", srv.latest)
		code, stdout, _ := runUpgrade(t, Options{Current: "1.0.4", Check: true, Pin: "v1.0.0"})
		if code != ExitUpgradeAvailable || stdout != "current 1.0.4\nrequested 1.0.0\n" {
			t.Fatalf("%d %q", code, stdout)
		}
		srv.setTag("v1.0.4", ghRelease{TagName: "v1.0.4"})
		code, stdout, _ = runUpgrade(t, Options{Current: "1.0.4", Check: true, Pin: "v1.0.4"})
		if code != 0 || stdout != "pogo 1.0.4 is already the requested version\n" {
			t.Fatalf("same pin %d %q", code, stdout)
		}
	})

	t.Run("no tty without yes", func(t *testing.T) {
		srv.reset(t, "v1.2.3", map[string][]byte{asset: []byte("nope")})
		code, _, stderr := runUpgrade(t, Options{Current: "1.0.0"})
		if code != 1 || !strings.Contains(stderr, "--yes") || !strings.Contains(stderr, "--check") {
			t.Fatalf("%d %s", code, stderr)
		}
		if srv.downloads != 0 {
			t.Fatal("downloaded without --yes")
		}
	})

	t.Run("missing asset", func(t *testing.T) {
		srv.reset(t, "v1.2.3", map[string][]byte{"SHA256SUMS": []byte("abc  other\n")})
		code, _, stderr := runUpgrade(t, Options{Current: "1.0.0", Yes: true})
		if code != 1 || !strings.Contains(stderr, asset) || !strings.Contains(stderr, releasesPage) {
			t.Fatalf("%d %s", code, stderr)
		}
	})

	t.Run("checksum mismatch", func(t *testing.T) {
		dir := t.TempDir()
		bin := filepath.Join(dir, "pogo")
		original := []byte("original-binary\n")
		if err := os.WriteFile(bin, original, 0o755); err != nil {
			t.Fatal(err)
		}
		executablePath = func() (string, error) { return bin, nil }
		blob := versionScript("1.2.3")
		sum := sha256.Sum256(blob)
		wrong := sha256.Sum256([]byte("not-the-blob"))
		if bytes.Equal(sum[:], wrong[:]) {
			t.Fatal("hash collided")
		}
		sums := hex.EncodeToString(wrong[:]) + "  " + asset + "\n"
		srv.reset(t, "v1.2.3", map[string][]byte{
			asset:        blob,
			"SHA256SUMS": []byte(sums),
		})
		code, _, stderr := runUpgrade(t, Options{Current: "1.0.0", Yes: true})
		if code != 1 || !strings.Contains(stderr, "SHA256 mismatch") {
			t.Fatalf("%d %s", code, stderr)
		}
		got, err := os.ReadFile(bin)
		if err != nil || !bytes.Equal(got, original) {
			t.Fatalf("binary changed %q %v", got, err)
		}
	})

	t.Run("upgrade and symlink", func(t *testing.T) {
		dir := t.TempDir()
		bin := filepath.Join(dir, "pogo")
		if err := os.WriteFile(bin, []byte("old\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(dir, "link")
		if err := os.Symlink(bin, link); err != nil {
			t.Fatal(err)
		}
		executablePath = func() (string, error) { return link, nil }
		blob := versionScript("1.2.3")
		sum := sha256.Sum256(blob)
		sums := fmt.Sprintf("%x  %s\n", sum, asset)
		srv.reset(t, "v1.2.3", map[string][]byte{asset: blob, "SHA256SUMS": []byte(sums)})
		code, stdout, stderr := runUpgrade(t, Options{Current: "1.0.0", Yes: true})
		if code != 0 || !strings.Contains(stdout, "Installed pogo 1.2.3 at "+bin) || !strings.Contains(stderr, "pogo 1.0.0 -> 1.2.3") {
			t.Fatalf("%d\n%s\n%s", code, stdout, stderr)
		}
		got, err := os.ReadFile(bin)
		if err != nil || !bytes.Equal(got, blob) {
			t.Fatalf("replaced %q %v", got, err)
		}
		if _, err := os.Lstat(link); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("pin older", func(t *testing.T) {
		dir := t.TempDir()
		bin := filepath.Join(dir, "pogo")
		if err := os.WriteFile(bin, []byte("current\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		executablePath = func() (string, error) { return bin, nil }
		blob := versionScript("1.0.0")
		sum := sha256.Sum256(blob)
		sums := fmt.Sprintf("%x  %s\n", sum, asset)
		rel := releaseWith(httpSrv.URL, "v1.0.0", map[string][]byte{asset: blob, "SHA256SUMS": []byte(sums)})
		srv.setFiles(rel, map[string][]byte{asset: blob, "SHA256SUMS": []byte(sums)})
		code, stdout, stderr := runUpgrade(t, Options{Current: "1.0.4", Yes: true, Pin: "v1.0.0"})
		if code != 0 || !strings.Contains(stdout, "Installed pogo 1.0.0") || !strings.Contains(stderr, "pogo 1.0.4 -> 1.0.0") {
			t.Fatalf("%d\n%s\n%s", code, stdout, stderr)
		}
	})

	t.Run("permission denied", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root ignores directory mode bits")
		}
		dir := t.TempDir()
		bin := filepath.Join(dir, "pogo")
		original := []byte("keep-me\n")
		if err := os.WriteFile(bin, original, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
		executablePath = func() (string, error) { return bin, nil }
		blob := versionScript("1.2.3")
		sum := sha256.Sum256(blob)
		sums := fmt.Sprintf("%x  %s\n", sum, asset)
		srv.reset(t, "v1.2.3", map[string][]byte{asset: blob, "SHA256SUMS": []byte(sums)})
		code, _, stderr := runUpgrade(t, Options{Current: "1.0.0", Yes: true})
		if code != 1 || !strings.Contains(stderr, "permission denied") || !strings.Contains(stderr, "sudo") {
			t.Fatalf("%d %s", code, stderr)
		}
		got, err := os.ReadFile(bin)
		if err != nil || !bytes.Equal(got, original) {
			t.Fatalf("changed %q %v", got, err)
		}
	})

	t.Run("check on nix store", func(t *testing.T) {
		oldEval := evalSymlinks
		t.Cleanup(func() { evalSymlinks = oldEval })
		evalSymlinks = func(p string) (string, error) { return p, nil }
		executablePath = func() (string, error) { return "/nix/store/abc/bin/pogo", nil }
		srv.reset(t, "v1.2.3", nil)
		code, stdout, stderr := runUpgrade(t, Options{Current: "1.0.4", Check: true})
		if code != ExitUpgradeAvailable || stdout != "current 1.0.4\nlatest 1.2.3\n" || strings.Contains(stderr, "Nix") {
			t.Fatalf("%d\n%s\n%s", code, stdout, stderr)
		}
		code, stdout, _ = runUpgrade(t, Options{Current: "1.2.3", Check: true})
		if code != 0 || stdout != "pogo 1.2.3 is the latest version\n" {
			t.Fatalf("current %d %q", code, stdout)
		}
		before := srv.downloads
		code, _, stderr = runUpgrade(t, Options{Current: "1.0.0", Yes: true})
		if code != 1 || !strings.Contains(stderr, "Nix") || !strings.Contains(stderr, "/nix/store/abc/bin/pogo") {
			t.Fatalf("install %d %s", code, stderr)
		}
		if srv.downloads != before {
			t.Fatal("nix install downloaded an asset")
		}
	})
}

func TestUpgradeWrapsTLSFailure(t *testing.T) {
	oldHook := executablePath
	t.Cleanup(func() { executablePath = oldHook })
	bin := filepath.Join(t.TempDir(), "pogo")
	if err := os.WriteFile(bin, []byte("untouched"), 0o755); err != nil {
		t.Fatal(err)
	}
	executablePath = func() (string, error) { return bin, nil }
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9"}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv(apiEnv, srv.URL)
	code, err := Run(Options{Current: "1.0.4", Check: true})
	if code != 1 || err == nil {
		t.Fatalf("%d %v", code, err)
	}
	msg := err.Error()
	for _, part := range []string{
		"pogo: could not reach GitHub to check for updates: certificate signed by unknown authority",
		"HTTPS_PROXY",
		"SSL_CERT_FILE",
		"https://github.com/JacksonJRay/pogo/releases/latest",
	} {
		if !strings.Contains(msg, part) {
			t.Fatalf("missing %q\n%s", part, msg)
		}
	}
	got, err := os.ReadFile(bin)
	if err != nil || string(got) != "untouched" {
		t.Fatalf("binary %q %v", got, err)
	}
}

func TestUpgradeDecline(t *testing.T) {
	oldHook, oldTerm := executablePath, stdinIsTerm
	t.Cleanup(func() {
		executablePath = oldHook
		stdinIsTerm = oldTerm
	})
	stdinIsTerm = func() bool { return true }
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, "n\n"); err != nil {
		t.Fatal(err)
	}
	w.Close()
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old; r.Close() })

	srv := &fakeRelease{}
	httpSrv := httptest.NewServer(srv)
	t.Cleanup(httpSrv.Close)
	srv.base = httpSrv.URL
	t.Setenv(apiEnv, httpSrv.URL)
	srv.reset(t, "v1.2.3", nil)
	dir := t.TempDir()
	bin := filepath.Join(dir, "pogo")
	if err := os.WriteFile(bin, []byte("old\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	executablePath = func() (string, error) { return bin, nil }
	code, _, stderr := runUpgrade(t, Options{Current: "1.0.0"})
	if code != 1 || !strings.Contains(stderr, "upgrade cancelled") || srv.downloads != 0 {
		t.Fatalf("%d %s downloads %d", code, stderr, srv.downloads)
	}
}

type fakeRelease struct {
	mu        sync.Mutex
	base      string
	latest    ghRelease
	tags      map[string]ghRelease
	files     map[string][]byte
	downloads int
	sawAuth   bool
	badUA     string
}

func (s *fakeRelease) reset(t *testing.T, tag string, files map[string][]byte) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.downloads = 0
	s.sawAuth = false
	s.badUA = ""
	s.tags = map[string]ghRelease{}
	s.files = files
	s.latest = releaseWith(s.base, tag, files)
}

func releaseWith(base, tag string, files map[string][]byte) ghRelease {
	rel := ghRelease{TagName: tag}
	for name := range files {
		rel.Assets = append(rel.Assets, ghAsset{
			Name:               name,
			BrowserDownloadURL: base + "/dl/" + name,
		})
	}
	return rel
}

func (s *fakeRelease) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Header.Get("Authorization") != "" {
		s.sawAuth = true
	}
	if strings.Contains(r.URL.Path, "/releases/") {
		if ua := r.Header.Get("User-Agent"); !strings.HasPrefix(ua, "pogo/") {
			s.badUA = ua
		}
		var rel ghRelease
		switch {
		case strings.HasSuffix(r.URL.Path, "/latest"):
			rel = s.latest
		default:
			tag := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			rel = s.tags[tag]
			if rel.TagName == "" {
				http.NotFound(w, r)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rel)
		return
	}
	name := filepath.Base(r.URL.Path)
	body, ok := s.files[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.downloads++
	_, _ = w.Write(body)
}

func (s *fakeRelease) setTag(tag string, rel ghRelease) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tags == nil {
		s.tags = map[string]ghRelease{}
	}
	s.tags[tag] = rel
}

func (s *fakeRelease) setFiles(rel ghRelease, files map[string][]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tags[rel.TagName] = rel
	s.files = files
	s.downloads = 0
}

func versionScript(version string) []byte {
	return []byte("#!/bin/sh\nif [ \"$1\" = version ]; then\nprintf '%s\\n' 'pogo " + version + "'\nexit 0\nfi\nexit 0\n")
}

func runUpgrade(t *testing.T, opts Options) (int, string, string) {
	t.Helper()
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	code, runErr := Run(opts)
	outW.Close()
	errW.Close()
	os.Stdout, os.Stderr = oldOut, oldErr
	stdout, _ := io.ReadAll(outR)
	stderr, _ := io.ReadAll(errR)
	outR.Close()
	errR.Close()
	if runErr != nil {
		stderr = append(stderr, []byte(runErr.Error()+"\n")...)
	}
	return code, string(stdout), string(stderr)
}
