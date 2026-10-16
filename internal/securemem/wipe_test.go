package securemem

import "testing"

func TestWipeZerosBuffer(t *testing.T) {
	b := []byte("super-secret-value")
	Wipe(b)
	for i, c := range b {
		if c != 0 {
			t.Fatalf("byte %d = %d, want 0", i, c)
		}
	}
	Wipe(nil)
	Wipe([]byte{})
}
