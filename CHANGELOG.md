# Changelog

## 2026-10-17 — 1.0.3

- When stdin is not a terminal and no key flag is set, Pogo asks through a prompt already on the machine. It tries `POGO_ASKPASS`, then the editor Git prompt (`VSCODE_GIT_IPC_HANDLE`, used by code-server, VS Code, and Cursor), then a desktop askpass when `WAYLAND_DISPLAY`, `DISPLAY`, or `MIR_SOCKET` is set. `gcr4-ssh-askpass` is used only when `GCR_SSH_ASKPASS_SOCKET` is present, because that program relays to GNOME and does not open a dialog by itself. A helper that cannot open a display is skipped. A real terminal still uses the echo-off prompt and waits until you type or press Ctrl-C. `POGO_TTY_TIMEOUT` is an optional number of seconds for a terminal nobody is watching. `--key-env` and `--key-stdin` are unchanged.
- If nothing can ask, Pogo exits 1. The first line is `pogo: no way to ask for the passphrase here`. The message lists what was checked and the ways forward, in order: a terminal, `docker run -it`, `--key-stdin` from a secret manager, `POGO_ASKPASS`, and `--key-env` as a last resort. The last line tells an agent not to ask for the passphrase in chat.
- `pogo doctor` reports which prompt would be used and does not ask. Exit 0 when one is available, exit 1 when not. `POGO_DEBUG=1` prints detection decisions on stderr.
- `--key-file` and `--new-key-file` stay rejected.

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
