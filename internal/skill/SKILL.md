# Pogo Skill

## What
Pogo is a single static binary that encrypts environment files with Argon2id + AES-256-GCM and injects the decrypted variables into a process at startup. The passphrase is never stored by Pogo.

## When to use
- A plaintext `.env` exists and should not remain on disk or be committed
- You need to edit secrets using a restricted temporary plaintext file
- You must start any server or application (Go, Node, Python, etc.) with those secrets
- You are an agent assisting a user with secret management

## Why
- No runtime dependencies
- Strong KDF (Argon2id 128 MiB / 4 iter / 4 threads) and authenticated encryption
- Works for every language via `pogo run --`
- Interactive and scripted workflows with explicit secret-handling limits
- Clear conversion path from existing `.env` files

## Convert an existing .env
```bash
pogo convert .env
# or with custom output name
pogo convert .env -f .env.production.enc
```

Pogo reads the plaintext file, prompts for a new passphrase (minimum 15 characters, entered twice), and writes the encrypted file. The original `.env` is left untouched. After verifying the encrypted file works, the user should delete the plaintext `.env`.

## Core workflows
```text
Convert:  pogo convert .env
Edit:     pogo edit
Run:      pogo run -- <command>          # uses ./.env.enc by default
Rekey:    pogo rekey
```

## Supplying the passphrase non-interactively
```text
--key-env POGO_KEY
--key-stdin
```

Never place the passphrase in shell history, logs, version control, or a file.
`--key-file` was removed. Do not write the passphrase to a file, and do not ask Pogo to read one.

## Rules for agents
- Run `pogo doctor` before starting a program. It does not ask for a passphrase.
- If doctor exits 0, run `pogo run -- <command>` with no key flags and tell the person to watch for the prompt. An agent runs `pogo run -- <command>` with no key flags. It does not collect the passphrase.
- If doctor exits 1, or Pogo prints `pogo: no way to ask for the passphrase here`, ask the person to run the command in their own terminal, or to pipe a secret manager into `--key-stdin`. Do not ask for the passphrase in chat. If you are an AI agent: do not ask the person to paste the passphrase into chat; ask them to run this command in their own terminal.
- The editor box is at the top of the window. code-server and VS Code show Pogo's prompt. Cursor's box can say "Sign in" and "Password or personal access token"; the title names the file (`pogo-passphrase-for-.env.enc.invalid`) and the username, if shown, is `pogo-passphrase`. That is still Pogo's passphrase. Do not enter a Git password or a personal access token.
- If Pogo prints `pogo: the editor prompt got no answer within the timeout`, the editor was asked and did not answer. Do not start another prompt and do not collect the passphrase. Ask the person to answer the box, or to run the command in their own terminal.
- `POGO_PROMPT_TIMEOUT` is a number of seconds for the editor, `POGO_ASKPASS`, and desktop dialogs. Unset, that wait is two minutes. `POGO_TTY_TIMEOUT` is only the terminal, and it is off unless set.
- `--key-env` is a last resort. The value stays visible in `/proc` and `docker inspect`.
- Prefer `pogo convert` when a plaintext `.env` already exists
- Show the exact command to the user before executing anything that writes or deletes
- Never echo, log, or store the passphrase
- Never write the passphrase to a file, even a mode-0600 file under /run, /tmp, or a Compose secret
- Never pass `--key-file` or `--new-key-file`. Pogo rejects them on purpose
- Do not pass `--allow-short-passphrase` unless the user explicitly asks for it, and only when creating a passphrase (`init`, `encrypt`, `convert`, or the new passphrase for `rekey`). A short passphrase is much easier to guess. The creation minimum is 15 characters. The flag skips that length check. A passphrase is still required, and an empty or whitespace-only passphrase is rejected. Opening an existing file does not use the minimum: pass the passphrase as it is, and do not add this flag
- Prefer the interactive prompt when a person is at the terminal
- Prefer `--key-stdin` over `--key-env`. Pipe the passphrase, for example `printf '%s' "$PASS" | pogo run --key-stdin -- cmd`, or `op read ... | pogo ...`. `--key-env` leaves the passphrase in the process environment, which is visible in `/proc` and `docker inspect`. Use `--key-env` only as a last resort
- `bash -lc` can reset `PATH`, so install pogo to `/usr/local/bin` or call it by absolute path
- After a successful convert, remind the user to remove the plaintext `.env`
- Use `pogo <command> --help` whenever flag details are needed. For containers, read `pogo help docker`
- Default encrypted filename is `.env.enc`; `pogo run -- <command>` uses it automatically
- Honor `-f` when the user specifies another name

## Short passphrases

Do not pass `--allow-short-passphrase` unless the user explicitly asks for a short passphrase. Short passphrases are much easier to guess. Pogo still requires a passphrase when creating one: empty or whitespace-only is rejected.

The 15-character minimum applies only when creating a passphrase: `init`, `encrypt`, `convert`, and the new passphrase for `rekey`. `--allow-short-passphrase` skips that minimum. One character that is not only whitespace is enough. It works with the prompt, `--key-stdin`, or `--key-env`, and it prints one warning on stderr.

Opening an existing file (`run`, `decrypt`, `edit`, and the current passphrase for `rekey`) accepts any non-empty passphrase. An empty passphrase is rejected. Do not pass `--allow-short-passphrase` on those commands; it is accepted and ignored, and it does not print a warning. `edit` re-saves with the same passphrase.

Example, only when the user asked for it:

```bash
printf '%s' "$SHORT_PASS" | pogo convert .env --key-stdin --allow-short-passphrase
```

Editing creates temporary plaintext. Cleanup covers normal exits and handled signals. SIGKILL, power loss, storage behavior, or editor backups can retain copies.
