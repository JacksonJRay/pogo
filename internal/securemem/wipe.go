// Package securemem zeroes secret buffers.
//
// Limits (also documented in the README): Go does not give a program a
// reliable way to erase secrets. The garbage collector can retain old
// copies, the compiler can theoretically elide stores it can prove are
// dead, and strings are immutable so they cannot be wiped. Argon2 and AES
// implementations copy key material into their own allocations. Wipe is a
// best-effort measure: it overwrites the caller's buffer and keeps that
// buffer live until the stores complete. It is not a promise that no copy
// remains in the process.
package securemem

import "runtime"

// Wipe overwrites b with zeros. A nil or empty buffer is a no-op.
// The compiler is discouraged from removing the stores by runtime.KeepAlive.
func Wipe(b []byte) {
	if len(b) == 0 {
		return
	}
	for i := range b {
		b[i] = 0
	}
	runtime.KeepAlive(b)
}
