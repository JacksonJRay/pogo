package upgrade

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestFallbackRoots(t *testing.T) {
	switch os.Getenv("POGO_ROOTS_CHILD") {
	case "example", "local":
		fallbackChild()
		return
	}

	emptyFile := filepath.Join(t.TempDir(), "empty.pem")
	if err := os.WriteFile(emptyFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	emptyDir := filepath.Join(t.TempDir(), "nocerts")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runRootsChild(t, "example", emptyFile, emptyDir, ""); err != nil {
		t.Fatalf("empty trust store should use Mozilla roots: %v", err)
	}

	caPEM, serverCert := testPair(t)
	other := filepath.Join(t.TempDir(), "other-ca.pem")
	if err := os.WriteFile(other, caPEM, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runRootsChild(t, "example", other, emptyDir, ""); err == nil {
		t.Fatal("a custom CA file was ignored in favor of the built-in roots")
	}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{serverCert}})
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})}
	go srv.Serve(ln)
	t.Cleanup(func() { _ = srv.Close() })
	if err := runRootsChild(t, "local", other, emptyDir, "https://"+ln.Addr().String()+"/"); err != nil {
		t.Fatalf("custom CA should verify the local server: %v", err)
	}
}

func runRootsChild(t *testing.T, mode, certFile, certDir, rawURL string) error {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestFallbackRoots$")
	cmd.Env = []string{
		"POGO_ROOTS_CHILD=" + mode,
		"POGO_ROOTS_URL=" + rawURL,
		"SSL_CERT_FILE=" + certFile,
		"SSL_CERT_DIR=" + certDir,
		"PATH=" + os.Getenv("PATH"),
		"GODEBUG=" + os.Getenv("GODEBUG"),
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, out)
	}
	return nil
}

func fallbackChild() {
	c := httpClient()
	c.Timeout = 20 * time.Second
	rawURL := "https://example.com/"
	if os.Getenv("POGO_ROOTS_CHILD") == "local" {
		rawURL = os.Getenv("POGO_ROOTS_URL")
	}
	resp, err := c.Get(rawURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		fmt.Fprintln(os.Stderr, resp.Status)
		os.Exit(1)
	}
	os.Exit(0)
}

func testPair(t *testing.T) ([]byte, tls.Certificate) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "pogo-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTpl, caTpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafTpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTpl, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	return caPEM, tls.Certificate{Certificate: [][]byte{leafDER}, PrivateKey: leafKey}
}
