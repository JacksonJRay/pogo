# Pogo

Encrypt `.env` files and load them into a program at startup.

Plaintext `.env` files sit on disk, in repos, and where agents can read them. Pogo is one static binary: it encrypts `.env` to `.env.enc`, and `pogo run` injects the variables at startup. No key file. Pogo does not store the passphrase.

## Install

Download `pogo-<os>-<arch>` and `SHA256SUMS` from the [latest release](https://github.com/JacksonJRay/pogo/releases/latest), check the hash, `chmod +x`, and put it on `PATH`. Once installed, `pogo upgrade`.

## Quick start

```bash
pogo init
pogo edit
pogo run -- npm start
pogo convert .env
```

## Commands
| Command | What it does |
|---|---|
| `pogo init [file]` | Create an encrypted file and open your editor |
| `pogo edit [file]` | Decrypt, edit, encrypt, delete the temp file |
| `pogo run [file] -- <command...>` | Decrypt in memory and exec the command |
| `pogo encrypt <plaintext> [file]` | Encrypt a plaintext file; the original stays |
| `pogo decrypt [file] [outfile]` | Decrypt to the terminal or a file (warns) |
| `pogo convert <plaintext.env> [file]` | Encrypt an existing env file; the original stays |
| `pogo rekey [file]` | Change the passphrase |
| `pogo upgrade` | Replace this binary, or `--check` |
| `pogo doctor` | Report the prompt and editor, without asking |
| `pogo skill` | Print the agent skill |

[Reference](docs/reference.md). [Docker](docs/docker.md).

The passphrase prompt opens out-of-band. Agents never see it. [docs/agents.md](docs/agents.md)

Security details: [docs/security.md](docs/security.md)

MIT. [LICENSE](LICENSE).
