# Pogo

Pogo encrypts env files and loads them into a program at startup. One static binary. No telemetry and no language SDK. Commands other than `pogo upgrade` do not use the network.

A passphrase is the long password for a file. Pogo does not save it and will not read it from a file. exec means Pogo replaces itself with your program, so Ctrl-C and the exit code belong to that program.

```bash
make build
export PATH="$PWD:$PATH"
pogo convert .env
pogo edit
pogo run -- npm start
```

`pogo` with no command prints the quick start and exits 1. `pogo help` prints the same page and exits 0.

## Security model

- AES-256-GCM encrypts the file. Argon2id turns the passphrase into the key, slowly, so guessing is expensive. You do not configure either one. The slow step uses 128 MiB, 4 passes, and 4 lanes (`golang.org/x/crypto/argon2`).
- Every save uses a new random 16-byte salt and a new random 12-byte nonce, so two saves of the same text do not look alike.
- The header (magic `POGO`, version, KDF parameters, salt, nonce) is authenticated with the ciphertext. A wrong passphrase or a flipped bit fails decryption (exit 2).
- The passphrase is never written to disk. There is no `--passphrase` flag.
- No telemetry. `pogo upgrade` is the only command that uses the network. It downloads a release over HTTPS and checks SHA256SUMS before it replaces the binary.
- Pogo zeroes passphrase and key bytes after use. Go can keep copies the program cannot reach. Zeroing is not a guarantee.
- Temp files for the editor are mode 0600 in a directory of mode 0700. Pogo prefers `$XDG_RUNTIME_DIR` when that directory is writable, then `/dev/shm`, then the system temp directory. It overwrites and removes them after a save, editor failure, a handled signal, or `POGO_EDIT_TIMEOUT`. SIGKILL, power loss, storage behavior, and editor-created copies can leave plaintext behind.
- Encrypted files are written by rename, mode 0600.
- Pogo tries to set `RLIMIT_CORE` to 0 so a crash is less likely to dump secrets. That can fail on some systems.
- Plaintext larger than 32 MiB is refused. Hostile headers cannot ask for more than 1 GiB of KDF memory, 100 iterations, or 32 lanes.

## Security verification and usability

The Pogo-only assessment and generated SBOM are in [security/README.md](security/README.md). The 2026-10-16 dependency scan reported zero CVE identifiers, with one unsuppressed module-only OpenPGP advisory. Source and binary analysis reported zero affected Pogo imports or calls. Run `make security` to repeat reachability checks against the current Go vulnerability database.

## Build

Go 1.26 or newer is required by the dependencies. The module selects the patched Go 1.27.2 toolchain for builds; Go downloads it when automatic toolchain selection is enabled. Targets: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64.

Release builds retain the symbol table for executable vulnerability analysis. `-trimpath` removes build-machine paths, `-buildvcs=false` omits local Git metadata, and `-w` removes debug information.

```bash
CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-w" -o pogo ./cmd/pogo
make test    # go vet and go test
make race
make security # source and binary govulncheck; downloads the pinned scanner
make security-report # regenerate the SBOM, scan reports, and artifact hashes
make e2e
make cross   # dist/pogo-linux-amd64, dist/pogo-linux-arm64, dist/pogo-darwin-amd64, dist/pogo-darwin-arm64
```

On macOS there is no `/dev/shm`. Pogo uses the system temp directory there. The darwin binaries are cross-compiled; exec and the temp-directory fallback are the same code paths as Linux.

Static check on the linux/amd64 binary from this tree (`file` and `ldd`):

```text
ELF 64-bit LSB executable, x86-64, version 1 (SYSV), statically linked, not stripped
not a dynamic executable
```

`dist/pogo-linux-arm64` is statically linked as well. The darwin binaries are Mach-O files built with `CGO_ENABLED=0`. `file` still reports `DYLDLINK` for those, because Go on macOS uses libSystem.

## Install and upgrade

Copy the binary for your OS and CPU somewhere on `PATH`, for example `/usr/local/bin`. The asset names are `pogo-linux-amd64`, `pogo-linux-arm64`, `pogo-darwin-amd64`, and `pogo-darwin-arm64`, next to `SHA256SUMS` on the GitHub release.

```bash
pogo upgrade --check
pogo upgrade
pogo upgrade --yes
pogo upgrade --version v1.0.3
```

`pogo upgrade` reads `https://api.github.com/repos/JacksonJRay/pogo/releases/latest` (no token unless `GITHUB_TOKEN` is set, and that token is sent only to `api.github.com`). It downloads `pogo-<os>-<arch>` and `SHA256SUMS` over HTTPS, using the same proxy settings as other programs (`HTTPS_PROXY`). It checks the SHA256 before it touches the installed binary. The new file's `version` command must print the expected version, and then Pogo renames it into place.

If this binary is already current, the output is `pogo 1.0.4 is the latest version` and the exit code is 0. Pogo does not install an older release unless `--version` names it. `--check` prints the versions and does not install: exit 0 when current, exit 10 when a different release would be installed. A terminal shows `current -> latest` and asks `Install? [y/N]`. With no terminal, pass `--yes`, or Pogo exits 1 and tells you to add it.

If the install directory is not writable, Pogo exits 1, leaves the original binary in place, and tells you to rerun with sudo or to move `pogo` somewhere you own. A Nix store path (`/nix/store/`) or a Guix store path (`/gnu/store/`) is left untouched. Upgrade that copy with the package manager. `POGO_UPGRADE_API` replaces the API address and is for tests. Leave it unset.

## Commands

| Command | What it does |
|---|---|
| `pogo init [file]` | Create a new encrypted file and open it in your editor |
| `pogo edit [file]` | Decrypt, open your editor, encrypt again, delete the temp file |
| `pogo run [file] -- <command...>` | Decrypt in memory and exec the command |
| `pogo encrypt <plaintext> [file]` | Encrypt a plaintext file |
| `pogo decrypt [file] [outfile]` | Decrypt to the terminal, or to a file if you pass one. Prints a warning |
| `pogo convert <plaintext.env> [file]` | Encrypt an existing env file. The original is left in place |
| `pogo rekey [file]` | Change the passphrase (new salt and nonce) |
| `pogo skill` | Print the agent skill |
| `pogo help [command]` | Help. Same as `pogo <command> --help` and `pogo <command> -h` |
| `pogo version` | Print the version. Same as `pogo -V` and `pogo --version` |
| `pogo docker` | Print the Docker guide. Same as `pogo help docker`. Does not start a container |
| `pogo upgrade` | Replace this binary with a GitHub release, or `--check` without installing |
| `pogo doctor` | Report which passphrase prompt would be used, and which editor `edit` would run, without asking |

The default encrypted file is `.env.enc`. `-f` / `--file` sets it on every command. A path typed after the command sets the same thing. If both are given and they differ, Pogo exits 1. For `decrypt`, one extra path is the plaintext output, and `-f` is the encrypted input.

### Flags

| Flag | Default | Meaning |
|---|---|---|
| `-f`, `--file` | `.env.enc` | Encrypted file. On `encrypt` and `convert`, the file Pogo writes |
| `--key-env NAME` | none | Last resort. Read the passphrase from that variable, exactly, with no trim. It stays in the process environment, so `/proc` and `docker inspect` can show it. `pogo run` removes `NAME` from the child. Prefer `--key-stdin` |
| `--key-stdin` | off | Pipe: read all of stdin, trim ends; the child sees end-of-file. Terminal: prompt on stderr, echo off, one line (same as the interactive prompt) |
| `--allow-short-passphrase` | off | When creating a passphrase, skip the 15-character minimum. Empty or whitespace-only still exits 4, and Pogo prints one warning on stderr. Opening an existing file ignores this flag |
| `--env NAME` | none | On `run` and `decrypt`, also load the `[NAME]` section. Those keys replace top-level keys with the same name |
| `--no-override` | off | On `run`, keep variables that are already set. By default the encrypted file wins |
| `--force` | off | On `init`, `encrypt`, `convert`, and `decrypt` to a file: replace an existing destination |
| `--new-key-env` | none | On `rekey`, read the new passphrase from a variable |
| `--list` | off | On `skill`, print framework names |
| `--framework NAME` | none | On `skill`, print one framework section |
| `--check` | off | On `upgrade`, compare versions and do not install. Exit 10 when a different release would be installed |
| `--yes` | off | On `upgrade`, install without asking. Required when stdin is not a terminal |
| `--version vX.Y.Z` | latest | On `upgrade` only, install that release, including an older one. `pogo --version` with no command still prints the running version |
| `-h`, `--help` |  | Help, exit 0 |

Use only one of `--key-env` and `--key-stdin`. If you pass neither and stdin is a terminal, Pogo asks in the terminal and waits until you type or press Ctrl-C. Set `POGO_TTY_TIMEOUT` to a number of seconds only when a terminal was allocated and nobody is there to type. Leave it unset for normal interactive use. Confirmation (typing the passphrase twice) happens for that prompt and for the out-of-band prompts below. `--key-env` and `--key-stdin` are still read once. `--key-file` and `--new-key-file` were removed: Pogo exits 1 and tells you to type the passphrase or use `--key-stdin`. A file that can decrypt the env file would be another secret stored on the system.

When stdin is not a terminal, Pogo asks through a prompt that is already on the machine. It stops at the first one that can show a dialog:

1. `POGO_ASKPASS`, if set. Same contract as `SSH_ASKPASS`: the prompt text is the argument, one line on stdout is the passphrase, exit 0 submits, any other exit cancels.
2. The editor Git prompt, when `VSCODE_GIT_IPC_HANDLE` is a live socket. code-server, VS Code, and Cursor inject this into integrated terminals, including Remote SSH and code-server behind a reverse proxy. A Flatpak or Snap editor whose socket path is not visible is looked up under `XDG_RUNTIME_DIR`. Stderr says `Pogo is asking for the .env.enc passphrase in the editor (box at the top of the window).` code-server and VS Code show that full prompt as the placeholder. Cursor uses its own box, titled from a fake host such as `pogo-passphrase-for-.env.enc.invalid` (the username, if shown, is `pogo-passphrase`). That host ends in `.invalid`, so it does not resolve and it is not a Git login. If the editor accepts the request and does not answer, Pogo waits and then exits 1 with `pogo: the editor prompt got no answer within the timeout`. It does not open a second dialog. A missing socket or a refused connection still falls through. `POGO_PROMPT_TIMEOUT` is a number of seconds for this wait and for `POGO_ASKPASS` and desktop dialogs. Unset, the wait is two minutes.
3. A desktop askpass already installed, and only when a display is actually there. A local `DISPLAY` of `:N` or `unix:N` is used only when `/tmp/.X11-unix/XN` exists. A `WAYLAND_DISPLAY` name is used only when that socket exists under `XDG_RUNTIME_DIR`, or when it is an absolute path that exists. TCP displays such as `localhost:10.0` (SSH forwarding) are not checked that way. If the variable is set and the local socket is missing, the check line is `display set but no X server socket`. PATH is searched first (NixOS, Alpine, Void, Gentoo), then the usual libexec paths (Debian, Ubuntu, Pop!_OS, Fedora, RHEL, Arch, openSUSE). `gcr4-ssh-askpass` and `gcr-ssh-askpass` only relay to GNOME's prompter, so Pogo uses them when `GCR_SSH_ASKPASS_SOCKET` is a socket and otherwise tries the next helper (zenity, kdialog, or another askpass). An SSH session or WSL without WSLg has no display, so this step is skipped. Pogo does not install packages, call `pkexec`, use `systemd-ask-password`, or run the editor's `askpass.sh`.

`pogo doctor` prints which of those would be used and does not ask. Exit 0 means a prompt is available. Exit 1 means it is not. `POGO_DEBUG=1` prints the same decisions on stderr while a command runs. It never prints the passphrase.

An agent runs `pogo doctor`, then `pogo run -- <command>` with no key flags, and tells the person to watch for the prompt. It does not collect the passphrase. If doctor exits 1, the person runs the command in their own terminal, or pipes a secret manager into `--key-stdin`. `--key-env` is a last resort: the value stays visible in `/proc` and `docker inspect`.

If nothing can ask, Pogo exits 1. The first line is `pogo: no way to ask for the passphrase here`.

```text
New passphrase for .env.enc:
Confirm new passphrase for .env.enc:
```

That pair is for creating a passphrase. The same two questions are asked through the terminal, `POGO_ASKPASS`, the editor box, or a desktop askpass. A mismatch exits 4 and writes nothing. Opening an existing file asks once. On a terminal that line is `Enter passphrase: `. `rekey` asks for the current passphrase, checks that it decrypts, and only then asks for the new one twice. Cursor titles those boxes from `pogo-passphrase-for-<file>.invalid`, `pogo-new-passphrase-for-<file>.invalid`, and `pogo-confirm-passphrase-for-<file>.invalid`. The username, if shown, stays `pogo-passphrase`.

### Multiple environments

Optional. Keys above the first `[section]` are the shared top. A `[name]` section is loaded only with `--env name`, and it replaces top-level keys with the same name. There is no syntax to return to the top level. `decrypt` without `--env` prints the original text, comments included. `decrypt --env NAME` prints the merged keys.

```text
PORT=43111
API_KEY=shared

[production]
API_KEY=prod-value
```

```bash
pogo run -- npm start
pogo run --env production -- npm start
```

Duplicate keys: the last one wins. Pogo does not expand `$VAR` inside values.

### Exit codes

| Code | Meaning |
|---|---|
| 0 | Success. For `pogo run`, once the program has started, this is the program's own exit code |
| 1 | Usage or other error: bad flag, missing file, cancelled prompt, no editor, program not found, interrupted edit, upgrade failed |
| 2 | Decryption failed: wrong passphrase or damaged file |
| 3 | The editor ran and exited non-zero, or the edit was not valid env syntax and there was no terminal to fix it. The encrypted file was not created or updated |
| 4 | Creating a passphrase: shorter than 15 characters, empty or only whitespace, or the two entries did not match. `--allow-short-passphrase` skips the 15-character minimum and still rejects a blank passphrase. Opening an existing file: only an empty passphrase |
| 10 | `pogo upgrade --check` only. A different release would be installed. Nothing was installed |

## Editing

On a terminal, the editor is `$POGO_EDITOR` if that is set, otherwise `$EDITOR`, otherwise `$VISUAL`, otherwise `vim`.

```bash
pogo edit
EDITOR=nano pogo edit
VISUAL=nano pogo init
EDITOR="vim -n" pogo edit
EDITOR="code --wait" pogo edit
POGO_EDITOR="code --wait" pogo edit
```

Without a terminal, an agent can still run `pogo edit`. The passphrase uses the usual prompt. The editor then opens where a person can see it, and Pogo waits until they save and close:

1. `$POGO_EDITOR`, if set. It is a command, so `code --wait` is allowed.
2. When `VSCODE_IPC_HOOK_CLI` is set (code-server, VS Code, VS Code Remote, Cursor), the editor's own CLI with `--wait`. Pogo looks next to `$VSCODE_GIT_ASKPASS_NODE` in `bin/remote-cli`, then for `code-server`, `code`, `cursor`, or `codium` on `PATH`. Stderr says `Pogo opened .env (decrypted) in the editor. Save and close the tab to continue.`
3. `$EDITOR` or `$VISUAL` when it is a GUI editor that waits (`code --wait`, `cursor --wait`, `gedit -s`, `subl -w`) or any other command that is not a terminal editor. `vim`, `nano`, `vi`, and `emacs -nw` need a terminal, so they are skipped. A script in `EDITOR` still runs. `xdg-open` does not wait, so it is skipped.
4. On a real display, a known waiting GUI editor on `PATH`. `xdg-open` is not used.

If none of those can open a window, Pogo exits 1, names what it checked, and does not print the decrypted file. `POGO_EDIT_TIMEOUT` is a number of seconds. Unset, Pogo waits until the editor exits. If the saved text is unchanged, `.env.enc` is not rewritten. The saved text must parse as an env file. Without a terminal, a syntax error exits 3 and writes nothing.

Pogo splits the editor value on spaces and adds the temp file as the last argument. It does not run a shell. VS Code and similar editors exit as soon as the window opens unless you pass their wait flag (`--wait` for VS Code). Pogo encrypts when the process exits.

In vim, `:wq` writes and `:q!` discards. In nano, Ctrl+O writes and Ctrl+X quits. Ctrl-C inside the editor cancels the editor command. It does not stop Pogo, because the editor is the foreground program. Stop the `pogo` process with SIGTERM, or quit the editor, to abort. The temp file is deleted and the encrypted file is not changed.

If an edit quits without changing the text, the encrypted file is left byte for byte as it was. `init` still writes the template if you do not change it, because that template is the new file. If your vim config writes backups outside the temp directory, those copies are plaintext. `vim -n` disables swap files. Disable backup and persistent undo separately, for example with `EDITOR="vim -n -i NONE -u NONE"`; cleanup cannot guarantee physical erasure.

## Examples

Node and Express, from `examples/node-express`:

```bash
pogo convert .env
pogo run -- npm start
```

A compiled Go server, from `examples/go-web`:

```bash
go build -o server .
pogo run -- ./server
pogo run -- go run .
```

SvelteKit, from `examples/sveltekit`. Use `$env/dynamic/private`. `$env/static/private` is fixed at `vite build`; a static import of a variable that was unset at build time fails the build, and starting the server later with Pogo does not fill it in.

```bash
pogo run -- npm run dev
npm run build
pogo run -- node build
```

One encrypted file, two apps: [examples/shared-env-demo/README.md](examples/shared-env-demo/README.md).

```bash
cd examples/shared-env-demo
cp .env.example .env
pogo convert .env
rm .env
cd python && pogo run -f ../.env.enc -- python3 app.py
cd ../web && pogo run -f ../.env.enc -- npm run dev
```

The same file also feeds a Node process in Docker. From `examples/shared-env-demo`, after `pogo convert` and `rm .env`, pipe the passphrase. There is no passphrase file:

```bash
read -rs POGO_PASS && printf '%s' "$POGO_PASS" | docker compose run -T --service-ports --rm --build node ; unset POGO_PASS
```

Flask is http://127.0.0.1:43121, SvelteKit is http://127.0.0.1:43122, and the container is http://127.0.0.1:43123. The walkthrough is [examples/shared-env-demo/README.md](examples/shared-env-demo/README.md).

## Docker

`pogo help docker` and `pogo run --help` print this guide. `pogo skill --framework docker` is the same material for an agent. Pogo will not read a passphrase from a file. Do not use a Compose secret for the passphrase.

`pogo run` uses exec, so the program is PID 1. `docker stop` signals that program, and `docker run` exits with the program's code. Do not use `--init` if the program itself must be PID 1. Mount `.env.enc` read-only. Dockerfiles for Alpine, scratch, and distroless are in `examples/docker/`. `examples/docker/pogo.Dockerfile` is a published-image layout (`COPY --from`). `examples/docker/pogo-src.Dockerfile` compiles Pogo in a Go stage and copies the binary onto scratch.

### Which method should I pick?

| You are... | Use | Security |
|---|---|---|
| A person at the keyboard | Interactive `docker run -it`, or `docker compose run`, or `docker compose up` then `docker attach` | Typed, not stored, not in `docker inspect` |
| A shell, once, at start | `--key-stdin` from `read -rs`, then `unset` | The shell variable exists until you unset it. Not in `docker inspect`. Stdin is consumed, so the app cannot also read stdin |
| Holding a password manager | `--key-stdin` from `op read` or `pass show` | Do not write that output to a file. Stdin is consumed. Not in `docker inspect` |
| Already exporting the variable | `--key-env` and `docker run -e POGO_KEY` | Last resort. `docker inspect` and `/proc` show it. Pogo removes `POGO_KEY` from the program it execs. Do not write the value in the command |
| Compose, a person | `docker compose run`, or `up` with `stdin_open: true` and `tty: true`, then `docker attach` | Same as interactive |
| Compose, a pipe | `docker compose run -T` with the `--key-stdin` entrypoint | Same as `--key-stdin`. `-T` is required or Compose allocates a terminal and the pipe is not the passphrase |

### 1. Interactive

When to use: a person is starting the container and can type.

Security: the passphrase is typed, not stored, and `docker inspect` does not show it.

```dockerfile
ENTRYPOINT ["pogo","run","-f","/app/.env.enc","--"]
CMD ["node","server.js"]
```

```bash
docker run -it --rm -v "$PWD/.env.enc:/app/.env.enc:ro" IMAGE
```

You will see this prompt. Nothing is echoed as you type:

```text
Enter passphrase:
```

Compose, attached terminal (same entrypoint, same prompt):

```bash
docker compose run --rm app
```

Compose up, then attach. The service needs `stdin_open: true` and `tty: true` (`examples/docker/compose.interactive.yaml`):

```bash
docker compose up -d
docker attach CONTAINER
```

Detach with Ctrl-P Ctrl-Q if the program should keep running.

### 2. `--key-stdin` from a prompt

When to use: you want to type it once in the shell, and Docker must not store it.

Security: the shell variable exists until you unset it. It is not in `docker inspect`. Stdin is consumed, so the program cannot also read stdin.

```dockerfile
ENTRYPOINT ["pogo","run","-f","/app/.env.enc","--key-stdin","--"]
CMD ["node","server.js"]
```

```bash
read -rs POGO_PASS && printf '%s' "$POGO_PASS" | docker run -i --rm -v "$PWD/.env.enc:/app/.env.enc:ro" IMAGE ; unset POGO_PASS
```

If that entrypoint is started with a terminal (`docker run -it`), Pogo prints `Enter passphrase: ` to stderr, turns echo off, and reads one line. You do not press Ctrl-D, and the passphrase is not echoed. That is the same as the interactive prompt. The program keeps the terminal. A pipe (`docker run -i`, or `docker compose run -T`) still reads all of stdin, trims the ends, and the program then sees end-of-file.

### 3. `--key-stdin` from a password manager

When to use: 1Password (`op`), `pass`, or another manager can print the passphrase.

Security: the manager prints it into the pipe. Do not write that output to a file. Stdin is consumed. `docker inspect` does not show it. The entrypoint is the same `--key-stdin` entrypoint as step 2.

```bash
op read op://vault/pogo/passphrase | docker run -i --rm -v "$PWD/.env.enc:/app/.env.enc:ro" IMAGE
pass show pogo | docker run -i --rm -v "$PWD/.env.enc:/app/.env.enc:ro" IMAGE
```

### 4. `--key-env`

Last resort. Prefer `--key-stdin` (`printf '%s' "$PASS" | pogo run --key-stdin -- cmd`, or `op read ... | pogo ...`). When to use: `POGO_KEY` is already in the environment of the command that starts Docker, and you accept the tradeoff.

Security: `docker inspect` shows `POGO_KEY`. It is also visible in `/proc` while the container runs. Pogo removes `POGO_KEY` from the program it execs, but Docker has already stored the value. Do not write the value in the command.

```dockerfile
ENTRYPOINT ["pogo","run","-f","/app/.env.enc","--key-env","POGO_KEY","--"]
CMD ["node","server.js"]
```

```bash
docker run --rm -e POGO_KEY -v "$PWD/.env.enc:/app/.env.enc:ro" IMAGE
```

### 5. Compose

When to use: you already have a compose file, including the shared-env demo.

Security: same as the method you pick. Do not use a Compose secret file, and do not put the passphrase in the compose file.

Interactive run uses the step 1 entrypoint. `docker compose run` allocates a terminal. Same prompt:

```bash
docker compose run --rm app
```

Interactive up uses `stdin_open: true` and `tty: true`, then:

```bash
docker compose up -d
docker attach CONTAINER
```

A pipe uses the step 2 entrypoint. `-T` means no terminal, so the pipe is stdin (`examples/docker/compose.stdin.yaml`):

```bash
read -rs POGO_PASS && printf '%s' "$POGO_PASS" | docker compose run -T --rm app ; unset POGO_PASS
op read op://vault/pogo/passphrase | docker compose run -T --rm app
```

### `--key-stdin` and a terminal

`docker run -it` on an image whose entrypoint is `pogo run -f /app/.env.enc --key-stdin --` prints the passphrase prompt, turns echo off, and reads one line. You do not press Ctrl-D, and the passphrase is not echoed. A pipe still uses `docker run -i` or `docker compose run -T`.

### Troubleshooting: no TTY

When stdin is not a terminal and nothing on the machine can ask, Pogo exits 1. The first line is `pogo: no way to ask for the passphrase here`. It lists what was checked. Ways forward, in order: run it in a terminal; `docker run -it`; `--key-stdin` from a secret manager; `POGO_ASKPASS`; `--key-env` as a last resort. The last line tells an agent not to ask for the passphrase in chat. `POGO_DEBUG=1` prints `pogo: stdin is not a terminal` and the same checks. If the editor was asked and did not answer, the first line is `pogo: the editor prompt got no answer within the timeout`, and Pogo does not open another dialog. `POGO_ASKPASS` and a desktop askpass use that same timeout wording (`pogo: the POGO_ASKPASS prompt got no answer within the timeout`, or the helper's name). A helper that exits non-zero, including Escape, still says the prompt was cancelled.

### systemd and Pogo on the host

systemd has no terminal. Pipe from a password manager, or use `--key-env` for that process only. `--key-env` is visible in `/proc` and in `systemctl show`. Do not point Pogo at a file.

```bash
ExecStart=/bin/sh -c 'op read op://vault/pogo/passphrase | /usr/local/bin/pogo run -f /etc/myapp/.env.enc --key-stdin -- /usr/bin/myapp'
```

Pogo on the host, starting the docker client, is separate. The client does not copy every variable into the container. Forward each name with `-e NAME` and no value. `sudo` drops the environment unless you pass `-E`. Export `POGO_KEY` in the shell first; do not put the passphrase in the command.

```bash
pogo run --key-env POGO_KEY -- docker run --rm -e API_KEY alpine printenv API_KEY
pogo run --key-env POGO_KEY -- sudo -E docker run --rm -e API_KEY alpine printenv API_KEY
pogo run --key-env POGO_KEY -- sudo -E docker compose -f examples/docker/compose.host.yaml up --abort-on-container-exit --exit-code-from passed
```

Compose interpolates `${VAR}` from the process Pogo started, and from a `.env` file next to the compose file. It does not forward every variable. `environment: - API_KEY` copies that one name. A name you leave out stays on the host. A value with a newline should use that list form, not an unquoted `${MULTILINE_VALUE}` in the YAML. `examples/docker/compose.host.yaml` is the worked example.

## Design decisions

- Header is 42 bytes, big-endian: magic, version 1, memory in KiB, iterations, parallelism, salt, nonce. The whole header is GCM additional data. Opening a file uses the parameters in the header, not today's defaults, within the hostile caps above.
- Production saves always use the default Argon2id parameters. Tests use a tiny parameter set.
- Default override: the encrypted file replaces variables already in the environment. `--no-override` keeps the existing value. For duplicate entries already in the environment, the first one wins, which is the POSIX rule.
- `[section]` plus `--env` is the reading of "several environments in one file." There is no return-to-top syntax.
- `--key-file` and `--new-key-file` were removed. Pogo will not read a passphrase from a file. `--key-env` does not trim, is stripped from the child, and is a last resort because the passphrase stays visible in `/proc` and `docker inspect`. `--key-stdin` on a pipe trims and consumes stdin. `--key-stdin` on a terminal prompts with echo off and reads one line.
- Passphrase length is counted in Unicode characters. Creating a passphrase (`init`, `encrypt`, `convert`, and the new passphrase for `rekey`) requires 15 characters. `--allow-short-passphrase` skips that minimum: one character that is not only whitespace is enough, empty or whitespace-only is still rejected, and Pogo prints one warning on stderr. The check runs before Argon2. Opening an existing file (`run`, `decrypt`, `edit`, and the current passphrase for `rekey`) accepts any non-empty passphrase and rejects only an empty one. `--allow-short-passphrase` is accepted on those commands and has no effect, and the short-passphrase warning is not printed. `edit` re-saves with the passphrase that opened the file and does not apply the creation minimum again.
- Maximum passphrase size is 4096 bytes.
- `pogo run` looks up the command on `PATH` (`exec.LookPath`) and then `syscall.Exec`. Defers do not run after a successful exec, so buffers are wiped first. The child's exit code and signals are the operating system's, not a wrapper's.
- `EDITOR` is split on whitespace, not by a shell. The temp path is one extra argument.
- `convert` does not delete the plaintext and does not offer to. It prints the `rm` command.
- Missing encrypted file is exit 1 with the next command to run. Bad magic or a wrong passphrase is exit 2.
- `rekey` checks the old passphrase, then the new one, and always writes a new salt and nonce.
- Dotenv: comments, blanks, `export `, quotes, double-quote escapes, multiline quotes, `=` inside values, CRLF, and a leading BOM. The stored payload is the original text. No `$VAR` expansion.
- Unknown commands and flags suggest a nearby name and point at `--help`.

## Verification

Run `make test` for Go vet and unit tests, `make race` for race checks, and
`make security` for current source and executable vulnerability analysis.
`make cross` compiles the four supported targets. `make e2e` exercises the
optional framework and Docker examples; install their prerequisites first.

Framework commands in the embedded skill are integration recipes. Verify them
against the framework versions used by your project. Password-manager examples
require the actual manager and a configured vault.

## License

MIT. See [LICENSE](LICENSE).
