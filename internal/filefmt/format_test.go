package filefmt

import (
	"bytes"
	"errors"
	"testing"
)

func fastParams() Params {
	return Params{Memory: 32, Iterations: 1, Parallelism: 1}
}

func TestRoundTripFast(t *testing.T) {
	pass := []byte("pogo-test-passphrase")
	plain := []byte("# comment\n\nA=b\n[prod]\nA=c\n")
	blob, err := seal(pass, plain, fastParams())
	if err != nil {
		t.Fatal(err)
	}
	out, err := Open(pass, blob)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, plain) {
		t.Fatalf("got %q", out)
	}
	h, _, err := Split(blob)
	if err != nil {
		t.Fatal(err)
	}
	if h.Params != fastParams() {
		t.Fatalf("header params = %+v", h.Params)
	}
	if h.Version != FormatVersion || len(h.Salt) != SaltLen || len(h.Nonce) != NonceLen {
		t.Fatalf("header = %+v", h)
	}
}

func TestTwoSealsDiffer(t *testing.T) {
	pass := []byte("pogo-test-passphrase")
	plain := []byte("A=b\n")
	a, err := seal(pass, plain, fastParams())
	if err != nil {
		t.Fatal(err)
	}
	b, err := seal(pass, plain, fastParams())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a, b) {
		t.Fatal("two seals were identical; salt or nonce was reused")
	}
}

func TestWrongPassphrase(t *testing.T) {
	blob, err := seal([]byte("pogo-test-passphrase"), []byte("A=b\n"), fastParams())
	if err != nil {
		t.Fatal(err)
	}
	_, err = Open([]byte("pogo-test-passphrase-nope"), blob)
	if !errors.Is(err, ErrDecrypt) {
		t.Fatalf("err = %v", err)
	}
	var de *Error
	if !errors.As(err, &de) || de.Kind != KindAuth {
		t.Fatalf("kind = %#v", err)
	}
}

func TestTamperCiphertextAndHeader(t *testing.T) {
	blob, err := seal([]byte("pogo-test-passphrase"), []byte("A=b\n"), fastParams())
	if err != nil {
		t.Fatal(err)
	}
	ct := append([]byte(nil), blob...)
	ct[len(ct)-1] ^= 0x01
	if _, err := Open([]byte("pogo-test-passphrase"), ct); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("ciphertext tamper err = %v", err)
	}
	hd := append([]byte(nil), blob...)
	hd[14] ^= 0x01 // salt byte, which is also AAD
	if _, err := Open([]byte("pogo-test-passphrase"), hd); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("header tamper err = %v", err)
	}
}

func TestHeaderRejection(t *testing.T) {
	if _, err := ParseHeader([]byte("POG")); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("short header: %v", err)
	}
	blob, err := seal([]byte("pogo-test-passphrase"), []byte("A=b\n"), fastParams())
	if err != nil {
		t.Fatal(err)
	}
	badMagic := append([]byte(nil), blob...)
	badMagic[0] = 'X'
	if _, err := Open([]byte("pogo-test-passphrase"), badMagic); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("bad magic: %v", err)
	}
	badVer := append([]byte(nil), blob...)
	badVer[4] = 99
	_, err = Open([]byte("pogo-test-passphrase"), badVer)
	if !errors.Is(err, ErrDecrypt) {
		t.Fatal(err)
	}
	var de *Error
	if !errors.As(err, &de) || de.Kind != KindFormat {
		t.Fatalf("version kind = %#v", err)
	}
	if _, err := Open([]byte("pogo-test-passphrase"), blob[:10]); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("truncated: %v", err)
	}
}

func TestOpenUsesHeaderParams(t *testing.T) {
	blob, err := seal([]byte("pogo-test-passphrase"), []byte("K=v\n"), fastParams())
	if err != nil {
		t.Fatal(err)
	}
	h, _, err := Split(blob)
	if err != nil {
		t.Fatal(err)
	}
	if h.Params == DefaultParams {
		t.Fatal("fast params unexpectedly equal defaults")
	}
	out, err := Open([]byte("pogo-test-passphrase"), blob)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "K=v\n" {
		t.Fatalf("got %q", out)
	}
}

func TestDefaultParamsMatchSpec(t *testing.T) {
	if DefaultParams.Memory != 128*1024 || DefaultParams.Iterations != 4 || DefaultParams.Parallelism != 4 {
		t.Fatalf("%+v", DefaultParams)
	}
	if testing.Short() {
		t.Skip("full Argon2id round trip")
	}
	pass := []byte("pogo-test-passphrase")
	plain := []byte("A=b\n")
	blob, err := Seal(pass, plain)
	if err != nil {
		t.Fatal(err)
	}
	h, _, err := Split(blob)
	if err != nil {
		t.Fatal(err)
	}
	if h.Params != DefaultParams {
		t.Fatalf("stored %+v", h.Params)
	}
	out, err := Open(pass, blob)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, plain) {
		t.Fatalf("got %q", out)
	}
}

func TestHostileParamsRejected(t *testing.T) {
	if err := validateParams(Params{Memory: 1 << 30, Iterations: 1, Parallelism: 1}); err == nil {
		t.Fatal("expected memory cap")
	}
	if err := validateParams(Params{Memory: 32, Iterations: 1000, Parallelism: 1}); err == nil {
		t.Fatal("expected iteration cap")
	}
}
