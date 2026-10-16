// Package filefmt implements Pogo's on-disk envelope.
//
// Layout (big-endian), version 1:
//
//	0:4    magic "POGO"
//	4      version (1)
//	5:9    Argon2id memory in KiB (uint32)
//	9:13   Argon2id iterations (uint32)
//	13     Argon2id parallelism (uint8)
//	14:30  salt (16 bytes)
//	30:42  nonce (12 bytes)
//	42:    AES-256-GCM ciphertext and tag
//
// The entire 42-byte header is GCM additional data, so a changed salt,
// nonce, version, or KDF parameter fails authentication.
package filefmt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
	"pogo/internal/securemem"
)

const (
	Magic         = "POGO"
	FormatVersion = byte(1)
	SaltLen       = 16
	NonceLen      = 12
	KeyLen        = 32
	headerLen     = 4 + 1 + 4 + 4 + 1 + SaltLen + NonceLen // 42
	gcmTagLen     = 16

	// MaxPlaintext is the largest decrypted payload Pogo will accept.
	MaxPlaintext = 32 << 20

	// Caps applied when reading a header so a corrupt or hostile file cannot
	// ask the process for an absurd Argon2 allocation. Spec files use 128 MiB,
	// 4 iterations, and 4 lanes, which sit inside these caps.
	maxMemoryKiB  = 1024 * 1024 // 1 GiB
	maxIterations = 100
	maxThreads    = 32
)

// ErrDecrypt is the parent of every error that means the file could not be
// decrypted. Callers map it to exit code 2.
var ErrDecrypt = errors.New("decryption failed")

// Kind classifies a decryption error for user-facing text.
type Kind int

const (
	KindAuth Kind = iota
	KindFormat
	KindParams
)

// Error is a decryption failure. Unwrap yields ErrDecrypt.
type Error struct {
	Kind Kind
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

func (e *Error) Unwrap() error { return ErrDecrypt }

func formatErr(msg string) error {
	return &Error{Kind: KindFormat, Msg: msg}
}

func authErr() error {
	return &Error{Kind: KindAuth, Msg: "authentication failed"}
}

func paramsErr(msg string) error {
	return &Error{Kind: KindParams, Msg: msg}
}

// Params are the Argon2id settings stored in the header.
type Params struct {
	Memory      uint32 // KiB
	Iterations  uint32
	Parallelism uint8
}

// DefaultParams are the v1 settings: 128 MiB, 4 iterations, 4 lanes.
var DefaultParams = Params{
	Memory:      128 * 1024,
	Iterations:  4,
	Parallelism: 4,
}

// Header is the parsed cleartext prefix of a Pogo file.
type Header struct {
	Version byte
	Params  Params
	Salt    []byte
	Nonce   []byte
	Raw     []byte
}

// Seal encrypts plaintext with a fresh salt and nonce using DefaultParams.
func Seal(passphrase, plaintext []byte) ([]byte, error) {
	return seal(passphrase, plaintext, DefaultParams)
}

func seal(passphrase, plaintext []byte, p Params) ([]byte, error) {
	if err := validateParams(p); err != nil {
		return nil, err
	}
	if len(plaintext) > MaxPlaintext {
		return nil, fmt.Errorf("plaintext is %d bytes; the limit is %d", len(plaintext), MaxPlaintext)
	}
	salt := make([]byte, SaltLen)
	nonce := make([]byte, NonceLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	if _, err := rand.Read(nonce); err != nil {
		securemem.Wipe(salt)
		return nil, err
	}
	defer securemem.Wipe(salt)
	defer securemem.Wipe(nonce)

	h := Header{
		Version: FormatVersion,
		Params:  p,
		Salt:    salt,
		Nonce:   nonce,
	}
	raw := h.Marshal()
	key, err := derive(passphrase, salt, p)
	if err != nil {
		securemem.Wipe(raw)
		return nil, err
	}
	defer securemem.Wipe(key)

	sealed, err := gcmSeal(key, nonce, plaintext, raw)
	if err != nil {
		securemem.Wipe(raw)
		return nil, err
	}
	out := make([]byte, 0, len(raw)+len(sealed))
	out = append(out, raw...)
	out = append(out, sealed...)
	return out, nil
}

// Open decrypts a file produced by Seal. KDF settings come from the header,
// not from the current defaults, so older files keep working.
func Open(passphrase, blob []byte) ([]byte, error) {
	h, ct, err := Split(blob)
	if err != nil {
		return nil, err
	}
	key, err := derive(passphrase, h.Salt, h.Params)
	if err != nil {
		return nil, err
	}
	defer securemem.Wipe(key)
	plain, err := gcmOpen(key, h.Nonce, ct, h.Raw)
	if err != nil {
		return nil, err
	}
	if len(plain) > MaxPlaintext {
		securemem.Wipe(plain)
		return nil, formatErr("decrypted payload is larger than the limit")
	}
	return plain, nil
}

// Split parses the header and returns the ciphertext (including the GCM tag).
func Split(blob []byte) (Header, []byte, error) {
	if len(blob) < headerLen+gcmTagLen {
		return Header{}, nil, formatErr("file is too short to be an encrypted env file")
	}
	h, err := ParseHeader(blob[:headerLen])
	if err != nil {
		return Header{}, nil, err
	}
	return h, blob[headerLen:], nil
}

// ParseHeader decodes a 42-byte header. The returned Header.Raw aliases data.
func ParseHeader(data []byte) (Header, error) {
	if len(data) < headerLen {
		return Header{}, formatErr("header is truncated")
	}
	raw := data[:headerLen]
	if string(raw[0:4]) != Magic {
		return Header{}, formatErr("file does not start with the Pogo magic bytes")
	}
	ver := raw[4]
	if ver != FormatVersion {
		return Header{}, formatErr(fmt.Sprintf("unsupported file version %d", ver))
	}
	p := Params{
		Memory:      binary.BigEndian.Uint32(raw[5:9]),
		Iterations:  binary.BigEndian.Uint32(raw[9:13]),
		Parallelism: raw[13],
	}
	if err := validateParams(p); err != nil {
		return Header{}, err
	}
	salt := make([]byte, SaltLen)
	nonce := make([]byte, NonceLen)
	copy(salt, raw[14:30])
	copy(nonce, raw[30:42])
	return Header{
		Version: ver,
		Params:  p,
		Salt:    salt,
		Nonce:   nonce,
		Raw:     raw,
	}, nil
}

// Marshal encodes h. Salt and nonce must already be the right length.
func (h Header) Marshal() []byte {
	b := make([]byte, headerLen)
	copy(b[0:4], Magic)
	b[4] = h.Version
	binary.BigEndian.PutUint32(b[5:9], h.Params.Memory)
	binary.BigEndian.PutUint32(b[9:13], h.Params.Iterations)
	b[13] = h.Params.Parallelism
	copy(b[14:30], h.Salt)
	copy(b[30:42], h.Nonce)
	return b
}

func validateParams(p Params) error {
	if p.Parallelism < 1 || p.Parallelism > maxThreads {
		return paramsErr(fmt.Sprintf("parallelism %d is outside 1..%d", p.Parallelism, maxThreads))
	}
	if p.Iterations < 1 || p.Iterations > maxIterations {
		return paramsErr(fmt.Sprintf("iterations %d is outside 1..%d", p.Iterations, maxIterations))
	}
	// Argon2id needs at least 8*lanes KiB. Mirror that floor so we never
	// store a header the KDF would silently bump.
	minMem := uint32(8) * uint32(p.Parallelism)
	if p.Memory < minMem || p.Memory > maxMemoryKiB {
		return paramsErr(fmt.Sprintf("memory %d KiB is outside %d..%d", p.Memory, minMem, maxMemoryKiB))
	}
	return nil
}

func derive(passphrase, salt []byte, p Params) ([]byte, error) {
	if len(salt) != SaltLen {
		return nil, formatErr("salt length is not 16 bytes")
	}
	return argon2.IDKey(passphrase, salt, p.Iterations, p.Memory, p.Parallelism, KeyLen), nil
}

func gcmSeal(key, nonce, plaintext, aad []byte) ([]byte, error) {
	aead, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	return aead.Seal(nil, nonce, plaintext, aad), nil
}

func gcmOpen(key, nonce, ciphertext, aad []byte) ([]byte, error) {
	aead, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	plain, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, authErr()
	}
	return plain, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != KeyLen {
		return nil, formatErr("derived key is not 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
