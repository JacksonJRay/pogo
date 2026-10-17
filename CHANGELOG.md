# Changelog

## 2026-10-17 — 1.0.2

- Opening an existing file (`run`, `decrypt`, `edit`, and the current passphrase for `rekey`) accepts any passphrase whose length is greater than 0. Only an empty passphrase is rejected (exit 4). A wrong passphrase still fails with exit 2. `--allow-short-passphrase` is still accepted on those commands and has no effect; Pogo does not print the short-passphrase warning on the open path. `edit` re-saves with the same passphrase and does not apply the creation minimum again.
- Creating a passphrase (`init`, `encrypt`, `convert`, and the new passphrase for `rekey`) still requires 15 characters. `--allow-short-passphrase` is the only way to skip that minimum (one non-whitespace character is enough). Empty or whitespace-only is still rejected, and the short-passphrase warning is still printed. The length check still runs before Argon2.
- Breaking: `--min-length` was removed.

## 2026-10-17 — 1.0.1

- Added `--allow-short-passphrase`. It skips the 15-character minimum on `init`, `edit`, `run`, `encrypt`, `decrypt`, `convert`, and `rekey` (the current passphrase and the new one). A passphrase is still required: empty or whitespace-only is rejected with exit 4. The flag prints one warning line on stderr: short passphrases are much easier to guess. That warning is not written to stdout.
- `--min-length` already accepts a number as low as 1 and warns below 15. The new flag is that same choice without a number. If both are set, `--min-length` is the minimum.
- Decryption of a file that already uses a passphrase of 15 or more characters is unchanged. Without the flag, a short passphrase still exits 4. A wrong passphrase still exits 2.
- `--key-file` and `--new-key-file` stay rejected with exit 1.

## 2026-10-16 — Pogo security maintenance

- Upgraded golang.org/x/crypto to v0.58.0, golang.org/x/sys to v0.49.0, and golang.org/x/term to v0.47.0.
- Selected Go 1.27.2 for patched builds and updated Pogo’s from-source Docker build.
- Retained release symbol tables for accurate executable vulnerability analysis.
- Added `make security`, a Pogo-only CycloneDX SBOM, unfiltered scan evidence, and a CLI usability assessment.
- Recorded zero current CVE identifiers and retained the module-only GO-2026-5932 OpenPGP advisory with zero reported reachable vulnerabilities.
- Added portable builds, reproducible security reports, and concise usage documentation.
