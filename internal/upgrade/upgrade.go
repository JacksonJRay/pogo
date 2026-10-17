// Package upgrade replaces the running pogo binary with a GitHub release.
//
// POGO_UPGRADE_API overrides the API origin (https://api.github.com). It is
// for tests. Downloads must use HTTPS. Plain HTTP is refused except for a
// localhost test server.
package upgrade

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"
)

const (
	repoAPI         = "/repos/JacksonJRay/pogo/releases"
	releasesPage    = "https://github.com/JacksonJRay/pogo/releases"
	defaultAPI      = "https://api.github.com"
	apiEnv          = "POGO_UPGRADE_API"
	maxReleaseBytes = 2 << 20
	maxSumsBytes    = 1 << 20
	maxBinaryBytes  = 64 << 20
	httpTimeout     = 2 * time.Minute
)

// ExitUpgradeAvailable is the status of `pogo upgrade --check` when a
// different release would be installed.
const ExitUpgradeAvailable = 10

// Options is one `pogo upgrade` invocation.
type Options struct {
	Current string
	Check   bool
	Yes     bool
	// Pin is the --version value, with or without a leading v. Empty means latest.
	Pin string
}

// executablePath is the binary to replace. Tests point it at a copy.
var executablePath = os.Executable

// stdinIsTerm reports whether the install question can be asked.
var stdinIsTerm = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

type semver struct {
	major, minor, patch int
	pre                 []string
}

func (v semver) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.major, v.minor, v.patch)
	if len(v.pre) > 0 {
		s += "-" + strings.Join(v.pre, ".")
	}
	return s
}

func (v semver) tag() string { return "v" + v.String() }

// compare returns -1, 0, or 1. Build metadata is ignored by parseSemver.
func compare(a, b semver) int {
	if a.major != b.major {
		return cmpInt(a.major, b.major)
	}
	if a.minor != b.minor {
		return cmpInt(a.minor, b.minor)
	}
	if a.patch != b.patch {
		return cmpInt(a.patch, b.patch)
	}
	switch {
	case len(a.pre) == 0 && len(b.pre) == 0:
		return 0
	case len(a.pre) == 0:
		return 1
	case len(b.pre) == 0:
		return -1
	}
	n := len(a.pre)
	if len(b.pre) < n {
		n = len(b.pre)
	}
	for i := 0; i < n; i++ {
		c := cmpIdent(a.pre[i], b.pre[i])
		if c != 0 {
			return c
		}
	}
	return cmpInt(len(a.pre), len(b.pre))
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func cmpIdent(a, b string) int {
	ai, aNum := atoiIdent(a)
	bi, bNum := atoiIdent(b)
	switch {
	case aNum && bNum:
		return cmpInt(ai, bi)
	case aNum:
		return -1
	case bNum:
		return 1
	}
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func atoiIdent(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

func parseSemver(raw string) (semver, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return semver{}, fmt.Errorf("version is empty")
	}
	s = strings.TrimPrefix(s, "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	var pre string
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre = s[i+1:]
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return semver{}, fmt.Errorf("version %q is not vX.Y.Z", raw)
	}
	nums := make([]int, 3)
	for i, p := range parts {
		if p == "" || (len(p) > 1 && p[0] == '0') {
			return semver{}, fmt.Errorf("version %q is not vX.Y.Z", raw)
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return semver{}, fmt.Errorf("version %q is not vX.Y.Z", raw)
		}
		nums[i] = n
	}
	var idents []string
	if pre != "" {
		idents = strings.Split(pre, ".")
		for _, id := range idents {
			if id == "" {
				return semver{}, fmt.Errorf("version %q is not vX.Y.Z", raw)
			}
			if _, num := atoiIdent(id); num && len(id) > 1 && id[0] == '0' {
				return semver{}, fmt.Errorf("version %q is not vX.Y.Z", raw)
			}
		}
	}
	return semver{major: nums[0], minor: nums[1], patch: nums[2], pre: idents}, nil
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	HTMLURL string    `json:"html_url"`
	Assets  []ghAsset `json:"assets"`
	Message string    `json:"message"`
}

// Run performs the upgrade and returns the process exit code.
// A non-nil error is the message for stderr. Status text is written to stdout.
func Run(opts Options) (int, error) {
	cur, err := parseSemver(opts.Current)
	if err != nil {
		return 1, fmt.Errorf("pogo: the running version %q is not vX.Y.Z.", opts.Current)
	}
	dest, err := resolveExecutable()
	if err != nil {
		return 1, err
	}

	base, err := apiBase()
	if err != nil {
		return 1, err
	}
	var pin semver
	pinned := opts.Pin != ""
	if pinned {
		pin, err = parseSemver(opts.Pin)
		if err != nil {
			return 1, fmt.Errorf("pogo: --version needs a release such as v1.2.3.\n%s", err.Error())
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), httpTimeout)
	defer cancel()
	client := httpClient()

	ua := "pogo/" + cur.String()
	var rel ghRelease
	if pinned {
		rel, err = fetchRelease(ctx, client, base+repoAPI+"/tags/"+url.PathEscape(pin.tag()), ua)
	} else {
		rel, err = fetchRelease(ctx, client, base+repoAPI+"/latest", ua)
	}
	if err != nil {
		return 1, err
	}
	target, err := parseSemver(rel.TagName)
	if err != nil {
		return 1, fmt.Errorf("pogo: the release tag %q is not vX.Y.Z.", rel.TagName)
	}
	if pinned && compare(target, pin) != 0 {
		return 1, fmt.Errorf("pogo: release %s did not match the requested %s.", target.tag(), pin.tag())
	}

	cmp := compare(cur, target)
	switch {
	case !pinned && cmp == 0:
		fmt.Fprintf(os.Stdout, "pogo %s is the latest version\n", cur.String())
		return 0, nil
	case !pinned && cmp > 0:
		fmt.Fprintf(os.Stdout, "pogo %s is newer than the latest release %s\n", cur.String(), target.String())
		return 0, nil
	case pinned && cmp == 0:
		fmt.Fprintf(os.Stdout, "pogo %s is already the requested version\n", cur.String())
		return 0, nil
	}
	if opts.Check {
		if pinned {
			fmt.Fprintf(os.Stdout, "current %s\nrequested %s\n", cur.String(), target.String())
		} else {
			fmt.Fprintf(os.Stdout, "current %s\nlatest %s\n", cur.String(), target.String())
		}
		return ExitUpgradeAvailable, nil
	}
	if tool, ok := managedBy(dest); ok {
		return 1, fmt.Errorf("pogo: this binary is in the %s store (%s).\nUpgrade pogo with %s. pogo upgrade will not change that path.", tool, dest, tool)
	}
	if !opts.Yes && !stdinIsTerm() {
		return 1, fmt.Errorf("pogo: no terminal, so %s was not installed.\nRun `pogo upgrade --yes` to install it, or `pogo upgrade --check` to compare versions.", target.String())
	}
	fmt.Fprintf(os.Stderr, "pogo %s -> %s\n", cur.String(), target.String())
	if !opts.Yes {
		ok, err := askInstall(os.Stdin)
		if err != nil {
			return 1, fmt.Errorf("pogo: could not read the install answer: %s", err.Error())
		}
		if !ok {
			return 1, fmt.Errorf("pogo: upgrade cancelled.\n%s was not installed.", target.String())
		}
	}

	assetName := "pogo-" + runtime.GOOS + "-" + runtime.GOARCH
	binURL, sumsURL, ok := assetURLs(rel, assetName)
	if !ok {
		return 1, fmt.Errorf("pogo: no %s binary for %s/%s.\nThe release assets are listed at %s", assetName, runtime.GOOS, runtime.GOARCH, releasesPage)
	}
	sums, err := fetchBytes(ctx, client, sumsURL, maxSumsBytes, false, ua)
	if err != nil {
		return 1, fmt.Errorf("pogo: could not download SHA256SUMS: %s", err.Error())
	}
	want, err := hashFor(sums, assetName)
	if err != nil {
		return 1, fmt.Errorf("pogo: %s", err.Error())
	}
	blob, err := fetchBytes(ctx, client, binURL, maxBinaryBytes, false, ua)
	if err != nil {
		return 1, fmt.Errorf("pogo: could not download %s: %s", assetName, err.Error())
	}
	sum := sha256.Sum256(blob)
	if subtle.ConstantTimeCompare(sum[:], want) != 1 {
		return 1, fmt.Errorf("pogo: SHA256 mismatch for %s.\nThe downloaded file was not installed.", assetName)
	}
	if err := install(dest, blob, target.String()); err != nil {
		return 1, err
	}
	fmt.Fprintf(os.Stdout, "Installed pogo %s at %s\n", target.String(), dest)
	return 0, nil
}

func askInstall(in io.Reader) (bool, error) {
	fmt.Fprint(os.Stderr, "Install? [y/N] ")
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func apiBase() (string, error) {
	base := defaultAPI
	if v := strings.TrimSpace(os.Getenv(apiEnv)); v != "" {
		base = strings.TrimRight(v, "/")
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", fmt.Errorf("pogo: %s is not a usable API address.", base)
	}
	if err := validateFetchURL(u); err != nil {
		return "", err
	}
	return base, nil
}

func validateFetchURL(u *url.URL) error {
	if u == nil {
		return fmt.Errorf("pogo: missing download URL.")
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		h := strings.ToLower(u.Hostname())
		if h == "localhost" || h == "127.0.0.1" || h == "::1" {
			return nil
		}
		return fmt.Errorf("pogo: refusing plain HTTP for %s.\nUpgrade downloads must use HTTPS. A localhost address is allowed only for tests.", u.Host)
	default:
		return fmt.Errorf("pogo: refusing URL scheme %q.", u.Scheme)
	}
}

func httpClient() *http.Client {
	var tr *http.Transport
	if base, ok := http.DefaultTransport.(*http.Transport); ok {
		tr = base.Clone()
	} else {
		tr = &http.Transport{Proxy: http.ProxyFromEnvironment}
	}
	tr.Proxy = http.ProxyFromEnvironment
	tr.DialContext = (&net.Dialer{Timeout: 10 * time.Second}).DialContext
	tr.TLSHandshakeTimeout = 10 * time.Second
	tr.ResponseHeaderTimeout = 30 * time.Second
	tr.IdleConnTimeout = 30 * time.Second
	return &http.Client{
		Timeout:   httpTimeout,
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return validateFetchURL(req.URL)
		},
	}
}

func fetchRelease(ctx context.Context, client *http.Client, rawURL, ua string) (ghRelease, error) {
	body, err := fetchBytes(ctx, client, rawURL, maxReleaseBytes, true, ua)
	if err != nil {
		return ghRelease{}, err
	}
	var rel ghRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return ghRelease{}, fmt.Errorf("pogo: the release response was not JSON.")
	}
	if rel.TagName == "" {
		msg := strings.TrimSpace(rel.Message)
		if msg == "" {
			msg = "the release response had no tag"
		}
		return ghRelease{}, fmt.Errorf("pogo: %s.\nSee %s", msg, releasesPage)
	}
	return rel, nil
}

func fetchBytes(ctx context.Context, client *http.Client, rawURL string, limit int64, api bool, ua string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("pogo: bad URL %q.", rawURL)
	}
	if err := validateFetchURL(u); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if ua == "" {
		ua = "pogo"
	}
	req.Header.Set("User-Agent", ua)
	if api {
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if token := os.Getenv("GITHUB_TOKEN"); token != "" && strings.EqualFold(u.Hostname(), "api.github.com") {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, reachErr(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("pogo: release was not found.\nSee %s", releasesPage)
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("pogo: GitHub refused the request (%s). Set GITHUB_TOKEN to raise the rate limit. The token is sent only to api.github.com.", resp.Status)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download failed: %s", resp.Status)
	}
	if resp.ContentLength > limit {
		return nil, fmt.Errorf("response is larger than the %d byte limit", limit)
	}
	buf, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, reachErr(err)
	}
	if int64(len(buf)) > limit {
		return nil, fmt.Errorf("response is larger than the %d byte limit", limit)
	}
	return buf, nil
}

func assetURLs(rel ghRelease, assetName string) (binURL, sumsURL string, ok bool) {
	for _, a := range rel.Assets {
		switch a.Name {
		case assetName:
			binURL = a.BrowserDownloadURL
		case "SHA256SUMS":
			sumsURL = a.BrowserDownloadURL
		}
	}
	return binURL, sumsURL, binURL != "" && sumsURL != ""
}

func hashFor(sums []byte, name string) ([]byte, error) {
	var found []byte
	for _, line := range bytes.Split(sums, []byte("\n")) {
		line = bytes.TrimRight(line, "\r")
		if len(bytes.TrimSpace(line)) == 0 || line[0] == '#' {
			continue
		}
		fields := bytes.Fields(line)
		if len(fields) != 2 {
			continue
		}
		fn := string(fields[1])
		fn = strings.TrimPrefix(fn, "*")
		if fn != name {
			continue
		}
		h, err := hex.DecodeString(string(fields[0]))
		if err != nil || len(h) != sha256.Size {
			return nil, fmt.Errorf("SHA256SUMS has a bad hash for %s", name)
		}
		found = h
	}
	if found == nil {
		return nil, fmt.Errorf("SHA256SUMS has no entry for %s.\nThe downloaded file was not installed.", name)
	}
	return found, nil
}

// evalSymlinks resolves the running binary. Tests can replace it.
var evalSymlinks = filepath.EvalSymlinks

func resolveExecutable() (string, error) {
	p, err := executablePath()
	if err != nil {
		return "", fmt.Errorf("pogo: could not find the running binary: %s", err.Error())
	}
	resolved, err := evalSymlinks(p)
	if err != nil {
		return "", fmt.Errorf("pogo: could not resolve the running binary %s: %s", p, err.Error())
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("pogo: could not resolve the running binary %s: %s", resolved, err.Error())
	}
	return abs, nil
}

func managedBy(path string) (string, bool) {
	clean := filepath.ToSlash(filepath.Clean(path))
	switch {
	case strings.Contains(clean, "/nix/store/"):
		return "Nix", true
	case strings.Contains(clean, "/gnu/store/"):
		return "Guix", true
	default:
		return "", false
	}
}

func install(dest string, blob []byte, version string) error {
	info, err := os.Stat(dest)
	if err != nil {
		return fmt.Errorf("pogo: could not stat %s: %s", dest, err.Error())
	}
	dir := filepath.Dir(dest)
	f, err := os.CreateTemp(dir, ".pogo-upgrade-*")
	if err != nil {
		return installErr(dest, err)
	}
	tmp := f.Name()
	keep := false
	defer func() {
		if !keep {
			f.Close()
			os.Remove(tmp)
		}
	}()
	if _, err := f.Write(blob); err != nil {
		return installErr(dest, err)
	}
	if err := f.Chmod(info.Mode().Perm()); err != nil {
		return installErr(dest, err)
	}
	if err := f.Sync(); err != nil {
		return installErr(dest, err)
	}
	if err := f.Close(); err != nil {
		return installErr(dest, err)
	}
	out, runErr := exec.Command(tmp, "version").Output()
	want := "pogo " + version + "\n"
	if runErr != nil || string(out) != want {
		got := strings.TrimSpace(string(out))
		if got == "" && runErr != nil {
			got = runErr.Error()
		}
		return fmt.Errorf("pogo: the downloaded binary did not report %q.\nIt reported %q.\nThe original binary was not changed.", "pogo "+version, got)
	}
	if err := os.Rename(tmp, dest); err != nil {
		return installErr(dest, err)
	}
	keep = true
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		d.Close()
	}
	return nil
}

func reachErr(err error) error {
	if err == nil || strings.Contains(err.Error(), "pogo:") {
		return err
	}
	return fmt.Errorf("pogo: could not reach GitHub to check for updates: %s\nCheck the network or proxy (HTTPS_PROXY). For a custom CA, set SSL_CERT_FILE. Or download the binary from https://github.com/JacksonJRay/pogo/releases/latest", shortReach(err))
}

func shortReach(err error) string {
	var unk x509.UnknownAuthorityError
	if errors.As(err, &unk) || strings.Contains(err.Error(), "unknown authority") {
		return "certificate signed by unknown authority"
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "timed out"
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "connection refused"):
		return "connection refused"
	case strings.Contains(msg, "no such host"):
		return "no such host"
	}
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	if len(msg) > 160 {
		msg = msg[:160]
	}
	return msg
}

func installErr(dest string, err error) error {
	if errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("pogo: cannot replace %s: permission denied.\nRun the same command with sudo, or move pogo to a directory you own and upgrade that copy.\nThe original binary was not changed.", dest)
	}
	return fmt.Errorf("pogo: cannot replace %s: %s.\nThe original binary was not changed.", dest, err.Error())
}
