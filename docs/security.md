# Security details

Threat model for the Pogo CLI. The dated dependency assessment and SBOM are in [../security/README.md](../security/README.md). Commands and flags are in [commands.md](commands.md). Editors and environment variables are in [reference.md](reference.md).

## Security model

- AES-256-GCM encrypts the file. Argon2id turns the passphrase into the key, slowly, so guessing is expensive. You do not configure either one. The slow step uses 128 MiB, 4 passes, and 4 lanes (`golang.org/x/crypto/argon2`).
- Every save uses a new random 16-byte salt and a new random 12-byte nonce, so two saves of the same text do not look alike.
- The header (magic `POGO`, version, KDF parameters, salt, nonce) is authenticated with the ciphertext. A wrong passphrase or a flipped bit fails decryption (exit 2).
- The passphrase is never written to disk. There is no `--passphrase` flag. `--key-file` and `--new-key-file` are rejected: a file that can decrypt the env file would be another secret stored on the system.
- No telemetry. `pogo upgrade` is the only command that uses the network. It downloads a release over HTTPS and checks SHA256SUMS before it replaces the binary.
- Pogo zeroes passphrase and key bytes after use. Go can keep copies the program cannot reach. Zeroing is not a guarantee.
- Temp files for the editor are mode 0600 in a directory of mode 0700. Pogo prefers `$XDG_RUNTIME_DIR` when that directory is writable, then `/dev/shm`, then the system temp directory. It overwrites and removes them after a save, editor failure, a handled signal, or `POGO_EDIT_TIMEOUT`. SIGKILL, power loss, storage behavior, and editor-created copies can leave plaintext behind.
- Encrypted files are written by rename, mode 0600.
- Pogo tries to set `RLIMIT_CORE` to 0 so a crash is less likely to dump secrets. That can fail on some systems.
- Plaintext larger than 32 MiB is refused. Hostile headers cannot ask for more than 1 GiB of KDF memory, 100 iterations, or 32 lanes.

## File format

- Header is 42 bytes, big-endian: magic, version 1, memory in KiB, iterations, parallelism, salt, nonce. The whole header is GCM additional data. Opening a file uses the parameters in the header, not today's defaults, within the hostile caps above.
- Production saves always use the default Argon2id parameters. Tests use a tiny parameter set.
- Passphrase length is counted in Unicode characters. Creating a passphrase (`init`, `encrypt`, `convert`, and the new passphrase for `rekey`) requires 15 characters. `--allow-short-passphrase` skips that minimum: one character that is not only whitespace is enough, empty or whitespace-only is still rejected, and Pogo prints one warning on stderr. The check runs before Argon2. Opening an existing file (`run`, `decrypt`, `edit`, and the current passphrase for `rekey`) accepts any non-empty passphrase and rejects only an empty one. `--allow-short-passphrase` is accepted on those commands and has no effect, and the short-passphrase warning is not printed. `edit` re-saves with the passphrase that opened the file and does not apply the creation minimum again.
- Maximum passphrase size is 4096 bytes.

## Verification

The Pogo-only assessment and generated SBOM are in [../security/README.md](../security/README.md). The 2026-10-16 dependency scan reported zero CVE identifiers, with one unsuppressed module-only OpenPGP advisory. Source and binary analysis reported zero affected Pogo imports or calls. Run `make security` to repeat reachability checks against the current Go vulnerability database.

`make security` runs source and binary `govulncheck`. `make security-report` regenerates the SBOM, scan reports, and artifact hashes. Build flags are in [reference.md](reference.md#build).
