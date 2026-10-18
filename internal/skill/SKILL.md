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

Pogo reads the plaintext file, prompts for a new passphrase (minimum 15 characters, entered twice), writes the encrypted file, and checks that the file decrypts. It then asks `Delete the original .env? [y/N]` on the controlling terminal. The default is No. Only `y` or `yes` deletes the plaintext. Enter, any other answer, EOF, and a session with no terminal keep the file. The question is not read from stdin, so `printf '%s' "$PASS" | pogo convert .env --key-stdin` can still ask on the terminal.

`--delete-original` deletes without asking. `--keep-original` keeps the file and does not ask. Either flag is for scripts. The delete is a normal remove, not a secure wipe. Do not pass `--delete-original` unless the user asked to delete the plaintext file. If conversion or the decrypt check fails, the plaintext is kept.

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
- Run `pogo doctor` before starting a program. It does not ask for a passphrase. `pogo doctor` covers init, edit, run, encrypt, decrypt, convert, and rekey.
- If doctor exits 0, run `pogo run -- <command>` with no key flags and tell the person to watch for the prompt. An agent runs `pogo run -- <command>` with no key flags. It does not collect the passphrase.
- To change the env file, run `pogo edit` with no key flags, tell the person a tab will open, and wait until pogo exits. Do not ask the person to paste values into chat.
- To see if a newer pogo exists, run `pogo upgrade --check`. To install it from an agent session, run `pogo upgrade --yes`. Do not downgrade unless the person names a version.
- If doctor exits 1, or Pogo prints `pogo: no way to ask for the passphrase here`, ask the person to run the command in their own terminal, or to pipe a secret manager into `--key-stdin`. Do not ask for the passphrase in chat. If you are an AI agent: do not ask the person to paste the passphrase into chat; ask them to run this command in their own terminal. Do not guess, try, or search the filesystem for a passphrase. Do not suggest printf 'PASSPHRASE' | pogo ..., because that stores the passphrase in shell history. Read it from a secret manager or keychain CLI instead.
- The editor box is at the top of the window. code-server and VS Code show Pogo's prompt. Cursor's box can say "Sign in" and "Password or personal access token"; the title names the file (`pogo-passphrase-for-.env.enc.invalid`) and the username, if shown, is `pogo-passphrase`. A new passphrase uses `pogo-new-passphrase-for-.env.enc.invalid`, and the confirmation uses `pogo-confirm-passphrase-for-.env.enc.invalid`. That is still Pogo's passphrase. Do not enter a Git password or a personal access token.
- Without a terminal, `pogo edit` opens a tab when `POGO_EDITOR` is set or when `VSCODE_IPC_HOOK_CLI` is set (code-server, VS Code, Cursor). The remote CLI search order is the bundled remote CLI, then `code-server`, `code`, `cursor`, `codium`, unless `TERM_PROGRAM` names one of those. Stderr says `Pogo opened .env.enc (decrypted) in the editor. Save and close the tab to continue (pogo waits up to 10 minutes; Ctrl+C to stop).` Wait for that tab to close. `POGO_EDIT_TIMEOUT` is seconds; `0` waits until the editor exits. If the person saved before the deadline, Pogo encrypts that text. If Pogo exits 1 because no editor can open the file, ask the person to run `pogo edit` in their own terminal or from the integrated terminal. Do not print or ask for the decrypted file. Do not guess, try, or search the filesystem for a passphrase.
- If Pogo prints `pogo: the editor prompt got no answer within the timeout`, the editor was asked and did not answer. The same kind of line is used when `POGO_ASKPASS` or a desktop askpass hits the timeout (`pogo: the POGO_ASKPASS prompt got no answer within the timeout`). Do not start another prompt and do not collect the passphrase. A helper that exits non-zero, including Escape, still says the prompt was cancelled. Ask the person to answer the box, or to run the command in their own terminal.
- `POGO_PROMPT_TIMEOUT` is a number of seconds for the editor, `POGO_ASKPASS`, and desktop dialogs. Unset, that wait is two minutes. `POGO_TTY_TIMEOUT` is only the terminal, and it is off unless set.
- `--key-env` is a last resort. The value stays visible in `/proc` and `docker inspect`.
- Prefer `pogo convert` when a plaintext `.env` already exists
- Show the exact command to the user before executing anything that writes or deletes
- Never echo, log, or store the passphrase
- Never write the passphrase to a file, even a mode-0600 file under /run, /tmp, or a Compose secret
- Never pass `--key-file` or `--new-key-file`. Pogo rejects them on purpose
- Do not pass `--allow-short-passphrase` unless the user explicitly asks for it, and only when creating a passphrase (`init`, `encrypt`, `convert`, or the new passphrase for `rekey`). A short passphrase is much easier to guess. The creation minimum is 15 characters. The flag skips that length check. A passphrase is still required, and an empty or whitespace-only passphrase is rejected. Opening an existing file does not use the minimum: pass the passphrase as it is, and do not add this flag
- Prefer the interactive prompt when a person is at the terminal
- Prefer `--key-stdin` over `--key-env`. Pipe a secret manager or keychain CLI, for example `op read ... | pogo run --key-stdin -- cmd` or `pass show ... | pogo ...`. Do not suggest printf 'PASSPHRASE' | pogo ..., because that stores the passphrase in shell history. `--key-env` leaves the passphrase in the process environment, which is visible in `/proc` and `docker inspect`. Use `--key-env` only as a last resort
- `bash -lc` can reset `PATH`, so install pogo to `/usr/local/bin` or call it by absolute path
- After a successful convert, Pogo checks that the encrypted file decrypts, then asks `Delete the original .env? [y/N]` on the controlling terminal. The default is No. Enter, anything other than y or yes, EOF, and no terminal keep the file. The question is read from the terminal, so it still works when `--key-stdin` has consumed stdin
- Do not pass `--delete-original` unless the user explicitly asks to delete the plaintext. `--keep-original` skips the question and leaves the file. Deleting is a normal remove, not a secure wipe. If there is no terminal, the plaintext file stays. Tell the user. Do not delete it yourself unless they asked
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
