# Reference

Landing page: [../README.md](../README.md). Agents and passphrase prompts: [agents.md](agents.md). Containers: [docker.md](docker.md). Threat model and file format: [security.md](security.md).

Shell examples in this file run from the repository root.

Pogo encrypts env files and loads them into a program at startup. One static binary. No telemetry and no language SDK. Commands other than `pogo upgrade` do not use the network.

A passphrase is the long password for a file. Pogo does not save it and will not read it from a file. exec means Pogo replaces itself with your program, so Ctrl-C and the exit code belong to that program.

`pogo` with no command prints the quick start and exits 1. `pogo help` prints the same page and exits 0.

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

Static check on the linux/amd64 binary from this tree (`file` and `ldd`), as recorded before this doc move:

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

If this binary is already the latest release, stdout is `pogo <version> is the latest version` (this tree: `pogo 1.0.4 is the latest version`) and the exit code is 0. If this binary is newer than the latest release, stdout is `pogo <version> is newer than the latest release <version>` and the exit code is 0. Nothing is installed in either case. Pogo does not install an older release unless `--version` names it. If `--version` names the version that is already running, stdout is `pogo <version> is already the requested version` and the exit code is 0.

`--check` compares and does not install. Exit 0 when this binary is current or newer than the latest release. Exit 10 when a different release would be installed. On that exit 10, the latest-release check prints `current <version>` and `latest <version>`. With `--version`, it prints `current <version>` and `requested <version>`. `pogo upgrade --check` still exits 0 or 10 on a Nix or Guix store path and does not change that path.

A terminal install prints `pogo <old> -> <new>` on stderr and asks `Install? [y/N]`. With no terminal, pass `--yes`, or Pogo exits 1 and tells you to add it.

If the install directory is not writable, Pogo exits 1, leaves the original binary in place, and tells you to rerun with sudo or to move `pogo` somewhere you own. A Nix store path (`/nix/store/`) or a Guix store path (`/gnu/store/`) is left untouched when an install would happen. Upgrade that copy with the package manager. `POGO_UPGRADE_API` replaces the API address and is for tests. Leave it unset. Plain HTTP is refused except for a localhost test server.

If the machine has no CA certificates, Pogo uses a built-in Mozilla root set so the HTTPS check can still succeed. System certificates are used when they exist. `SSL_CERT_FILE` and `SSL_CERT_DIR` still select a custom CA. If GitHub cannot be reached, Pogo exits 1, leaves the binary untouched, and prints `pogo: could not reach GitHub to check for updates:` plus a short reason, with advice to check the network or `HTTPS_PROXY`, set `SSL_CERT_FILE` for a custom CA, or download from https://github.com/JacksonJRay/pogo/releases/latest.

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

The default encrypted file is `.env.enc`. `-f` / `--file` sets it on every command that takes a file. A path typed after the command sets the same thing. If both are given and they differ, Pogo exits 1. For `decrypt`, one extra path is the plaintext output, and `-f` is the encrypted input. `skill`, `help`, `version`, `upgrade`, `docker`, and `doctor` do not read an encrypted file. `help` takes an optional command name. `upgrade`, `docker`, and `doctor` reject extra arguments. `skill` and `version` ignore extra words.

`init` and `convert` are two ways to create the encrypted file. `init` writes a commented template and opens the editor. `convert` reads an existing plaintext env file. Running the second of them against an existing `.env.enc` needs `--force`.

## Flags

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
| `-V`, `--version` |  | With no command, print the running version and exit 0. On `upgrade`, `--version` is the release to install. `-V` is not the pin |

Use only one of `--key-env` and `--key-stdin`. If you pass neither, Pogo asks. The prompt order (terminal, `POGO_ASKPASS`, the editor, a desktop dialog) is in [agents.md](agents.md). `--key-file` and `--new-key-file` were removed: Pogo exits 1 and tells you to type the passphrase or use `--key-stdin`.

Which flags each command accepts:

| Command | Flags besides `-h` / `--help` |
|---|---|
| `init` | `-f`, `--key-env`, `--key-stdin`, `--allow-short-passphrase`, `--force` |
| `edit` | `-f`, `--key-env`, `--key-stdin`, `--allow-short-passphrase` |
| `run` | `-f`, `--key-env`, `--key-stdin`, `--allow-short-passphrase`, `--env`, `--no-override` |
| `encrypt` | `-f`, `--key-env`, `--key-stdin`, `--allow-short-passphrase`, `--force` |
| `decrypt` | `-f`, `--key-env`, `--key-stdin`, `--allow-short-passphrase`, `--env`, `--force` |
| `convert` | `-f`, `--key-env`, `--key-stdin`, `--allow-short-passphrase`, `--force` |
| `rekey` | `-f`, `--key-env`, `--key-stdin`, `--allow-short-passphrase`, `--new-key-env` |
| `skill` | `--list`, `--framework` |
| `upgrade` | `--check`, `--yes`, `--version` |
| `help`, `version`, `docker`, `doctor` | none |

## Multiple environments

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

## Exit codes

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

Without a terminal, an agent can still run `pogo edit`. The passphrase uses the usual prompt. The editor then opens where a person can see it. The order is `$POGO_EDITOR`, then the editor CLI when `VSCODE_IPC_HOOK_CLI` is set, then `$EDITOR` or `$VISUAL` when that command can run without a terminal, then a known waiting GUI editor on a real display. Details, including code-server and Cursor: [agents.md](agents.md#editing-without-a-terminal).

Pogo splits the editor value on spaces and adds the temp file as the last argument. It does not run a shell. VS Code and similar editors exit as soon as the window opens unless you pass their wait flag (`--wait` for VS Code). Pogo encrypts when the process exits.

In vim, `:wq` writes and `:q!` discards. In nano, Ctrl+O writes and Ctrl+X quits. Ctrl-C inside the editor cancels the editor command. It does not stop Pogo, because the editor is the foreground program. Stop the `pogo` process with SIGTERM, or quit the editor, to abort. The temp file is deleted and the encrypted file is not changed.

If an edit quits without changing the text, the encrypted file is left byte for byte as it was. `init` still writes the template if you do not change it, because that template is the new file. If your vim config writes backups outside the temp directory, those copies are plaintext. `vim -n` disables swap files. Disable backup and persistent undo separately, for example with `EDITOR="vim -n -i NONE -u NONE"`; cleanup cannot guarantee physical erasure.

The temp file is mode 0600 in a directory of mode 0700. Pogo prefers `$XDG_RUNTIME_DIR` when that directory is writable, then `/dev/shm`, then the system temp directory. It overwrites and removes the file after a save, editor failure, a handled signal, or `POGO_EDIT_TIMEOUT`.

`POGO_EDIT_TIMEOUT` is a number of seconds. `0` waits until the editor exits. Unset, a terminal waits until the editor exits. Unset with no terminal and a GUI or remote-CLI editor, Pogo waits up to 10 minutes. If the file was saved before that deadline, Pogo encrypts the saved text and says so. The saved text must parse as an env file. On a terminal, a syntax error opens the editor again. Without a terminal, a syntax error exits 3 and writes nothing.

## Environment variables

| Variable | Role |
|---|---|
| `POGO_EDITOR` | Editor command. Overrides `EDITOR` and `VISUAL`. Split on spaces. No shell |
| `EDITOR`, `VISUAL` | Editor when `POGO_EDITOR` is unset. On a terminal, the default is `vim` |
| `POGO_EDIT_TIMEOUT` | Seconds to wait for the editor. `0` waits until it exits. See Editing |
| `POGO_ASKPASS` | Askpass program. Same contract as `SSH_ASKPASS`. See [agents.md](agents.md) |
| `POGO_PROMPT_TIMEOUT` | Seconds for the editor prompt, `POGO_ASKPASS`, and desktop dialogs. Unset: two minutes |
| `POGO_TTY_TIMEOUT` | Seconds for a terminal prompt when nobody is there to type. Unset for normal use |
| `POGO_DEBUG` | `1` prints prompt and editor decisions on stderr. Never prints the passphrase |
| `POGO_UPGRADE_API` | Test-only replacement for the GitHub API address. Leave unset |
| `GITHUB_TOKEN` | Sent as a bearer token only to `api.github.com`, and only when set |
| `HTTPS_PROXY` | Proxy for the upgrade download |
| `SSL_CERT_FILE`, `SSL_CERT_DIR` | Custom CA for the upgrade download |
| `XDG_RUNTIME_DIR` | Preferred directory for editor temp files, when writable |
| `VSCODE_GIT_IPC_HANDLE` | Editor socket used for the passphrase box |
| `VSCODE_IPC_HOOK_CLI` | Set by code-server, VS Code, and Cursor. Selects the remote CLI for `edit` |
| `VSCODE_GIT_ASKPASS_NODE` | Used to find `bin/remote-cli` |
| `TERM_PROGRAM` | Which of `code-server`, `code`, `cursor`, or `codium` to try first |
| `DISPLAY`, `WAYLAND_DISPLAY`, `MIR_SOCKET` | Desktop prompt. Local X and Wayland need a socket. A TCP `DISPLAY` counts without one. `MIR_SOCKET` counts when the path exists |
| `GCR_SSH_ASKPASS_SOCKET` | Required before `gcr-ssh-askpass` or `gcr4-ssh-askpass` is used |

## Examples

Node and Express, from `../examples/node-express`:

```bash
pogo convert .env
pogo run -- npm start
```

A compiled Go server, from `../examples/go-web`:

```bash
go build -o server .
pogo run -- ./server
pogo run -- go run .
```

SvelteKit, from `../examples/sveltekit`. Use `$env/dynamic/private`. `$env/static/private` is fixed at `vite build`; a static import of a variable that was unset at build time fails the build, and starting the server later with Pogo does not fill it in.

```bash
pogo run -- npm run dev
npm run build
pogo run -- node build
```

One encrypted file, two apps: [../examples/shared-env-demo/README.md](../examples/shared-env-demo/README.md).

```bash
cd examples/shared-env-demo
cp .env.example .env
pogo convert .env
rm .env
cd python && pogo run -f ../.env.enc -- python3 app.py
cd ../web && pogo run -f ../.env.enc -- npm run dev
```

The same file also feeds a Node process in Docker. From `examples/shared-env-demo`, after `pogo convert` and `rm .env`, pipe the passphrase. There is no passphrase file. The shell variable is what `printf` writes; do not put the passphrase text in the command.

```bash
read -rs POGO_PASS && printf '%s' "$POGO_PASS" | docker compose run -T --service-ports --rm --build node ; unset POGO_PASS
```

Flask is http://127.0.0.1:43121, SvelteKit is http://127.0.0.1:43122, and the container is http://127.0.0.1:43123. The walkthrough is [../examples/shared-env-demo/README.md](../examples/shared-env-demo/README.md). The container recipes are in [docker.md](docker.md).

## Design decisions

- Default override: the encrypted file replaces variables already in the environment. `--no-override` keeps the existing value. For duplicate entries already in the environment, the first one wins, which is the POSIX rule.
- `[section]` plus `--env` is the reading of "several environments in one file." There is no return-to-top syntax.
- `--key-file` and `--new-key-file` were removed. Pogo will not read a passphrase from a file. `--key-env` does not trim, is stripped from the child, and is a last resort because the passphrase stays visible in `/proc` and `docker inspect`. `--key-stdin` on a pipe trims and consumes stdin. `--key-stdin` on a terminal prompts with echo off and reads one line.
- `pogo run` looks up the command on `PATH` (`exec.LookPath`) and then `syscall.Exec`. Defers do not run after a successful exec, so buffers are wiped first. The child's exit code and signals are the operating system's, not a wrapper's.
- `EDITOR` is split on whitespace, not by a shell. The temp path is one extra argument.
- `convert` does not delete the plaintext and does not offer to. It prints the `rm` command. `encrypt` does the same.
- Missing encrypted file is exit 1 with the next command to run. Bad magic or a wrong passphrase is exit 2.
- `rekey` checks the old passphrase, then the new one, and always writes a new salt and nonce.
- Dotenv: comments, blanks, `export `, quotes, double-quote escapes, multiline quotes, `=` inside values, CRLF, and a leading BOM. The stored payload is the original text. No `$VAR` expansion.
- Unknown commands and flags suggest a nearby name and point at `--help`.

Header layout, Argon2id parameters, and the hostile caps are in [security.md](security.md).

## Verification

Run `make test` for Go vet and unit tests, `make race` for race checks, and
`make security` for current source and executable vulnerability analysis.
`make cross` compiles the four supported targets. `make e2e` exercises the
optional framework and Docker examples; install their prerequisites first.

Framework commands in the embedded skill are integration recipes. Verify them
against the framework versions used by your project. Password-manager examples
require the actual manager and a configured vault.
