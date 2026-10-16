package passphrase

import "testing"

func TestCheckLength(t *testing.T) {
	if err := CheckLength([]byte("short-pass"), DefaultMin); err != ErrTooShort {
		t.Fatalf("%v", err)
	}
	if err := CheckLength([]byte("pogo-test-passphrase"), DefaultMin); err != nil {
		t.Fatal(err)
	}
	if err := CheckLength([]byte("pogo-test-passphrase"), 21); err != ErrTooShort {
		t.Fatalf("custom min: %v", err)
	}
	// 15 runes, more than 15 bytes.
	if err := CheckLength([]byte("äääääääääääääää"), DefaultMin); err != nil {
		t.Fatal(err)
	}
	long := make([]byte, MaxBytes+1)
	for i := range long {
		long[i] = 'a'
	}
	if err := CheckLength(long, 1); err != ErrTooLong {
		t.Fatalf("%v", err)
	}
}

func TestMatch(t *testing.T) {
	if !Match([]byte("abc"), []byte("abc")) {
		t.Fatal("equal")
	}
	if Match([]byte("abc"), []byte("abd")) || Match([]byte("abc"), []byte("abcd")) {
		t.Fatal("unequal")
	}
}

func TestReadEnvExact(t *testing.T) {
	t.Setenv("POGO_TEST_PASS", "pogo-test-passphrase")
	got, err := Acquire(Source{Env: "POGO_TEST_PASS"})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "pogo-test-passphrase" {
		t.Fatalf("%q", got)
	}
	if _, err := Acquire(Source{Env: "POGO_TEST_PASS_MISSING"}); err == nil {
		t.Fatal("expected missing env error")
	}
}
