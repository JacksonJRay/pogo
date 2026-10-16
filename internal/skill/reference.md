
## Command and flag reference

Words used below:

- A passphrase is a long password you type. Pogo never writes it down, and it will not read one from a file.
- An encrypted file is the `.env.enc` (or `-f`) file. It is ciphertext, safe to keep on disk. The passphrase is not.
- exec means Pogo replaces itself with your program. The program keeps the same process id, so Ctrl-C and the exit code belong to that program.
- KDF means key derivation: turning a passphrase into an encryption key. Argon2id is the slow method Pogo uses, so guessing is expensive. AES-256-GCM is the encryption. You do not choose or tune either one.

| Command | What it does |
|---|---|
| `pogo init [file]` | Create a new encrypted file and open it in your editor |
| `pogo edit [file]` | Decrypt, open your editor, encrypt again, delete the temp file |
| `pogo run [file] -- <command...>` | Decrypt and exec the command with the variables set |
| `pogo encrypt <plaintext> [file]` | Encrypt a plaintext file |
| `pogo decrypt [file] [outfile]` | Decrypt to the terminal or to a file (prints a warning) |
| `pogo convert <plaintext.env> [file]` | Encrypt an existing `.env`. The original file is left in place |
| `pogo rekey [file]` | Change the passphrase |
| `pogo skill` | Print this document |
| `pogo help [command]` | Show help. Same as `pogo <command> --help` and `pogo <command> -h` |
| `pogo version` | Print the version |

Flags on every command:

| Flag | Default | Meaning |
|---|---|---|
| `-f`, `--file` | `.env.enc` | Encrypted file path. On `encrypt` and `convert` this is the output |
| `--key-env` | none | Read the passphrase from this environment variable. Exact value, no trim. `pogo run` removes it from the child |
| `--key-stdin` | off | Read all of stdin, trim whitespace, and use that as the passphrase |
| `--min-length` | 15 | Minimum passphrase length in characters. Lowest allowed value is 1 |
| `-h`, `--help` |  | Show help and exit 0 |

Other flags:

| Flag | Commands | Meaning |
|---|---|---|
| `--env NAME` | `run`, `decrypt` | Use the `[NAME]` section, merged over the top of the file |
| `--no-override` | `run` | Keep variables that are already set. By default the encrypted file wins |
| `--force` | `init`, `encrypt`, `convert`, `decrypt` | Replace an existing destination file |
| `--new-key-env` | `rekey` | Read the new passphrase from an environment variable. `--new-key-file` was removed |
| `--list` | `skill` | Print framework names, one per line |
| `--framework NAME` | `skill` | Print only that framework's section |

If both `-f` and a positional encrypted-file path are given and they differ, Pogo stops (exit 1). Use only one of them.

`pogo run` requires `--` before the program:

```bash
pogo run -- npm start
pogo run -f .env.production.enc -- ./server
```

## Choosing how to supply the passphrase

Use one method only. If you pass more than one of `--key-env` and `--key-stdin`, Pogo stops. `--key-file` and `--new-key-file` were removed: Pogo exits 1 and tells you to type the passphrase or use `--key-stdin`.

| Situation | Use | Why |
|---|---|---|
| You are at a terminal | Type it when asked | Nothing is stored. You type it twice when creating or rekeying |
| Docker, a person | `docker run -it` or `docker compose run` | Same prompt. See `pogo help docker` |
| A pipe, password manager, or `read -rs` | `--key-stdin` | Stdin is consumed. The program sees end-of-file. Not shown by `docker inspect` |
| A process already has the variable | `--key-env POGO_KEY` | Pogo removes it from the child. `docker inspect` and `/proc` still show it if you passed `-e` |
| An agent or script | `--key-stdin` or the prompt | Never write the passphrase to a file |

Interactive prompts:

```text
Enter passphrase (min 15 characters):
Enter it again to confirm:
```

`rekey` asks for the current passphrase, then the new one, then confirmation.

Never put the passphrase in shell history, logs, source control, a file, or command-line arguments. There is no `--passphrase` flag on purpose. Do not write it to a Compose secret or `/run/secrets` for Pogo to read.

`pogo help docker` and `pogo skill --framework docker` are the step-by-step container recipes: interactive `-it`, `--key-stdin` from `read -rs` or a password manager, `--key-env`, and Compose (`docker compose run` versus `docker compose run -T`).

`--key-stdin` reads the entire stdin and trims leading and trailing whitespace. `--key-env` uses the variable's exact value, with no trim. Typed passphrases are used exactly as entered (the terminal already strips the newline).

When you create a file (`init`, `convert`, `encrypt`) or set a new passphrase (`rekey`), interactive entry is confirmed by typing it twice. `--key-env` and `--key-stdin` are not confirmed a second time. Pogo will not read a passphrase from a file.

The minimum length is checked on `init`, `edit`, `encrypt`, `decrypt`, `convert`, `rekey`, and `run`, before any slow key derivation. A short passphrase exits 4. If you encrypted with `--min-length 12`, later commands must pass `--min-length 12` as well, or they will refuse the passphrase.

## Multiple environments

One encrypted file can hold a shared top and named sections. This is optional. A file with no `[section]` lines behaves like a normal env file.

```text
# loaded for every run
PORT=43111
API_KEY=shared

[production]
API_KEY=prod-value

[dev]
API_KEY=dev-value
```

```bash
pogo run -- npm start                  # PORT and API_KEY=shared
pogo run --env production -- npm start # PORT and API_KEY=prod-value
pogo decrypt --env dev                 # prints the merged dev view
```

Rules:

- Put shared keys before the first `[name]`.
- `[name]` uses letters, digits, `.`, `_`, and `-`.
- A section replaces top-level keys with the same name and adds its own keys.
- Without `--env`, section keys are not loaded.
- If the name is missing, Pogo lists the sections it found and exits 1.
- `decrypt` without `--env` prints the original file, comments included. `decrypt --env NAME` prints only the merged keys.

## Override behavior

By default, values from the encrypted file replace variables that are already set in the environment. That way the file is what the program sees.

```bash
pogo run -- ./server
pogo run --no-override -- ./server
```

`--no-override` keeps an existing value and fills in only variables that are unset.

The variable named by `--key-env` is removed from the program's environment so the passphrase is not passed on. If the encrypted file itself contains that same name, the file's value is still set.

Child processes inherit the environment. `npm start`, `pnpm`, `yarn`, nodemon, `go run`, and shell scripts all see the variables, and so do programs they spawn.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success. For `pogo run`, this is the program's own exit code when the program exits 0 |
| 1 | Usage or other error (bad flag, missing file, passphrase mismatch, program not found) |
| 2 | Decryption failed: wrong passphrase or damaged file |
| 3 | The editor ran and exited non-zero. The encrypted file was not updated |
| 4 | Passphrase shorter than `--min-length` (default 15) |

`pogo run` uses exec. Once the program has started, the exit code and signals are the program's, not Pogo's. Pogo's own codes above apply only when Pogo fails before the program starts. Example: a program that exits 7 makes the shell see 7.

## Editing the file yourself

`pogo init` and `pogo edit` open a normal editor on a temporary copy of the env file. You edit that copy the same way you edit any text file. When the editor exits 0, Pogo encrypts the result. You do not learn a special editor.

Which program opens:

1. `$EDITOR`, if it is set and not blank.
2. Otherwise `$VISUAL`.
3. Otherwise `vim`.

```bash
pogo edit
EDITOR=nano pogo edit
VISUAL=nano pogo init
EDITOR="vim -n" pogo edit
EDITOR="code --wait" pogo edit
```

`vim -n` is vim plus an argument (do not keep a swap file). `code --wait` is the same shape: program, arguments, and Pogo adds the file path at the end.

Pogo splits that value on spaces. It does not run a shell. Do not put pipes, redirects, or quotes in `EDITOR`. If you need a shell, point `EDITOR` at a script file. Pogo passes the temp path as the last argument (`$1` in a shell script).

Graphical editors (VS Code, Sublime, and similar) often exit as soon as the window opens. Pogo encrypts when the process exits, so tell the editor to wait until you close the window. For VS Code the flag is `--wait`. Without it, the encrypted file is written before you finish typing.

Saving and discarding:

- In vim, `:wq` writes and quits. `:q!` quits without writing.
- In nano, Ctrl+O writes and Ctrl+X quits. Ctrl+X then `n` discards changes.
- Inside vim or nano, Ctrl-C cancels the current editor command. It does not quit the editor, and it does not stop Pogo, because the editor is the foreground program.
- To abort Pogo, quit the editor, or stop the `pogo` process (SIGTERM, or Ctrl-C if Pogo is the foreground program). Pogo deletes the temp file and does not change the encrypted file. Run `pogo edit` again when you want to change it.
- If an edit quits with the text unchanged, Pogo leaves the encrypted file byte for byte as it was and prints `No changes`.
- `init` still creates the file from the template if you do not change the template. The template is the new file.
- If the editor exits non-zero (vim `:cq`, or a script that exits 1), Pogo exits 3 and does not create or update the encrypted file.

Where the plaintext lives while you edit:

- A directory with mode 0700, and a file named `env` with mode 0600.
- On Linux that directory is under `/dev/shm` when `/dev/shm` exists and is writable (a memory disk, not the physical disk).
- Otherwise it is under the system temp directory. macOS has no `/dev/shm`, so Pogo uses the temp directory there.
- Pogo attempts to overwrite and remove the file after a save, editor failure, or handled interrupt. SIGKILL, power loss, storage behavior, and editor-created copies can leave plaintext behind.
- The temporary file uses `/dev/shm` when available; other processes and editor settings can create additional copies.

If your vim config writes backup copies to another folder, those copies are plaintext and Pogo cannot delete them. `vim -n` disables swap files. Disable backup and persistent undo separately, for example with `EDITOR="vim -n -i NONE -u NONE"`.

Check a required variable without printing its value:

```bash
pogo run -- sh -c 'test -n "$API_KEY" && printf "API_KEY is set\n"'
```

## Troubleshooting

Wrong passphrase or damaged file (exit 2):

```text
Decryption failed. The passphrase was wrong, or the file was changed since it was encrypted.
```

Check the file path (`-f`) and the passphrase. After `pogo rekey`, the old passphrase no longer works.

A variable the program cannot see:

- It is under a `[section]` and you did not pass `--env`.
- It is already set in the environment and you passed `--no-override`.
- The line is a comment, or the dotenv line failed to parse (Pogo exits 1 and names the line).
- You used `--key-env SOME_NAME` and also expected the passphrase variable to remain. Pogo removes it.
- The framework only exposes variables to the browser when they have a special prefix, or only when they were present at build time. See the framework sections below.

Frontend prefixes:

- Vite exposes `VITE_` variables to client code, and it inlines them when the dev server or the build starts. Other variables stay on the server only if you read them from Node.
- Next.js exposes `NEXT_PUBLIC_` to the browser, inlined when the client bundle is compiled. Other variables are available in server code (`process.env`) at runtime.
- SvelteKit `$env/dynamic/private` reads the real environment when the server handles a request. `$env/static/private` is copied in at `vite build` time. Building without Pogo, then starting the built server with Pogo, fills dynamic variables and leaves static ones at whatever was set during the build.

`--key-stdin` and programs that read stdin: the passphrase ate stdin. Type the passphrase at the prompt instead. Do not switch to a key file.

`pogo run npm start` without `--` is treated as a file name, not a program. Put the program after `--`.

The editor exited with an error (exit 3): the encrypted file was not changed. Quit the editor with a normal save (`:wq` in vim, Ctrl+O then Ctrl+X in nano) so it exits 0. A GUI editor that was missing `--wait` can also exit before you save; set `EDITOR="code --wait"` and run `pogo edit` again.

Pogo says it stopped before saving (exit 1): the temp file is already deleted. The encrypted file was not changed. Run `pogo edit` again.
