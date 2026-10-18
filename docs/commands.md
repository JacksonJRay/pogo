# Commands

One section per command in Pogo 1.0.5. Prompts and agents: [agents.md](agents.md). Exit codes, editors, environment variables, and examples: [reference.md](reference.md). Containers: [docker.md](docker.md). Threat model: [security.md](security.md).

`pogo` with no command prints the quick start and exits 1. `pogo help` prints the same page and exits 0.

The default encrypted file is `.env.enc`. On commands that take a file, `-f` / `--file` and a positional path set the same thing. If both are given and they differ, Pogo exits 1.

Use only one of `--key-env` and `--key-stdin`. If you pass neither, Pogo asks. `--key-file` and `--new-key-file` are rejected (exit 1). There is no `--passphrase` flag. Pogo will not read a passphrase from a file.

Creating a passphrase (`init`, `encrypt`, `convert`, and the new passphrase for `rekey`) asks twice, requires 15 characters, and exits 4 if it is too short, empty or only whitespace, or the two entries do not match. `--allow-short-passphrase` skips the 15-character minimum, still rejects a blank passphrase, and prints one warning on stderr. Opening an existing file asks once. An empty passphrase is exit 4. A wrong passphrase or a damaged file is exit 2. `--allow-short-passphrase` is accepted on open and does nothing.

Every command accepts `-h` / `--help` (exit 0). `-V` / `--version` with no command prints the running version. On `upgrade` only, `--version vX.Y.Z` chooses the release. `pogo upgrade -V` is an error.

## init

`pogo init [file]`

Create an encrypted file and open it in your editor. Pogo writes a short commented template, encrypts whatever you save, and still writes that template if you do not change it. The editor must exit 0 or Pogo does not create the file (exit 3). `--force` replaces an existing destination.

Flags: `-f` / `--file`, `--key-env`, `--key-stdin`, `--allow-short-passphrase`, `--force`.

```bash
pogo init
pogo init -f .env.production.enc
```

## edit

`pogo edit [file]`

Decrypt, open your editor, encrypt again, and delete the temp file. If the text did not change, the encrypted file is left byte for byte as it was. The saved text must parse as an env file. On a terminal, a syntax error opens the editor again. Without a terminal, a syntax error exits 3 and writes nothing. An editor that exits non-zero does not update the file (exit 3).

Flags: `-f` / `--file`, `--key-env`, `--key-stdin`, `--allow-short-passphrase`. `--allow-short-passphrase` has no effect: `edit` re-saves with the passphrase that opened the file.

```bash
pogo edit
EDITOR=nano pogo edit
```

Editor selection, `POGO_EDITOR`, and `POGO_EDIT_TIMEOUT`: [reference.md](reference.md#editing). Without a terminal: [agents.md](agents.md#editing-without-a-terminal).

## run

`pogo run [file] -- <command> [args...]`

Decrypt in memory and exec the command. The program replaces Pogo, so Ctrl-C and the exit code belong to that program. No plaintext file is written. The command is required and must come after `--`. Words before `--` are an encrypted-file path, not the program.

`--env NAME` also loads the `[NAME]` section. Those keys replace top-level keys with the same name. `--no-override` keeps variables that are already set. By default the encrypted file wins. Sections: [reference.md](reference.md#multiple-environments).

Flags: `-f` / `--file`, `--key-env`, `--key-stdin`, `--allow-short-passphrase`, `--env`, `--no-override`.

```bash
pogo run -- npm start
pogo run -f .env.production.enc -- ./server
pogo run --env production -- go run .
```

`pogo run` removes `--key-env`'s variable from the child. `--key-stdin` on a pipe consumes stdin. Containers: [docker.md](docker.md).

## encrypt

`pogo encrypt <plaintext> [file]`

Encrypt a plaintext env file. The original is left in place. Pogo prints an `rm` command. It does not delete the plaintext. The plaintext path and the encrypted path must differ. `--force` replaces an existing destination.

Flags: `-f` / `--file`, `--key-env`, `--key-stdin`, `--allow-short-passphrase`, `--force`.

```bash
pogo encrypt .env
pogo encrypt .env -f .env.production.enc
```

## decrypt

`pogo decrypt [file] [outfile]`

Decrypt to the terminal, or to a file if you pass one. Pogo always prints a warning. Prefer `pogo run`. `-` also means the terminal. If `-f` is set and you pass one positional path, that path is the output file. `--env NAME` prints the merged keys for that section, not the raw file. Without `--env`, the original text is printed, comments included. `--force` replaces an existing output file. Pogo refuses to write the plaintext onto the encrypted file.

Flags: `-f` / `--file`, `--key-env`, `--key-stdin`, `--allow-short-passphrase`, `--env`, `--force`.

```bash
pogo decrypt
pogo decrypt .env.enc ./check.env
pogo decrypt --env production
```

## convert

`pogo convert <plaintext.env> [file]`

Encrypt an existing env file. The two paths must differ. `--force` replaces an existing destination. `init` and `convert` are two ways to create the encrypted file. The second one needs `--force` if `.env.enc` is already there.

After the encrypted file is written, Pogo reads it back and decrypts it. Only then does it ask `Delete the original <file>? [y/N]` on the controlling terminal. The default is No. Only `y` or `yes` deletes the plaintext. Enter, any other answer, EOF, and a session with no terminal keep the file. The question is not read from stdin, so it still works when `--key-stdin` is a pipe. `--delete-original` removes the plaintext without asking. `--keep-original` leaves it and does not ask. A failed conversion or a failed decrypt check never deletes the plaintext. The remove is a normal delete, not a secure wipe. `encrypt` does not ask and does not delete.

Flags: `-f` / `--file`, `--key-env`, `--key-stdin`, `--allow-short-passphrase`, `--force`, `--delete-original`, `--keep-original`.

```bash
pogo convert .env
pogo convert .env -f .env.production.enc
```

## rekey

`pogo rekey [file]`

Change the passphrase. Pogo asks for the current passphrase, checks that it decrypts, and only then asks for the new one twice. The file is written again with a new salt and nonce. The old passphrase stops working. `--new-key-env NAME` reads the new passphrase from that variable (once, not twice). `--key-env` on `rekey` is the current passphrase.

Flags: `-f` / `--file`, `--key-env`, `--key-stdin`, `--allow-short-passphrase`, `--new-key-env`. `--allow-short-passphrase` applies only to the new passphrase.

```bash
pogo rekey
pogo rekey --key-env POGO_OLD --new-key-env POGO_NEW
```

## upgrade

`pogo upgrade [--check] [--yes] [--version vX.Y.Z]`

Replace this binary with a GitHub release, or compare without installing. No passphrase and no env file. Download, hash check, Nix and Guix, and the exact messages: [reference.md](reference.md#install-and-upgrade).

`--check` does not install. Exit 0 when this binary is current or newer than the latest release. Exit 10 when a different release would be installed. `--yes` installs without asking and is required when stdin is not a terminal. `--version vX.Y.Z` installs that release, including an older one. This is the only downgrade.

Flags: `--check`, `--yes`, `--version`. No positional arguments.

```bash
pogo upgrade --check
pogo upgrade
pogo upgrade --yes
pogo upgrade --version v1.0.3
```

## doctor

`pogo doctor`

Report which passphrase prompt would be used, and which editor `edit` and `init` would run, without asking. Exit 0 when a prompt is available. Exit 1 when nothing can ask. The editor lines do not change the exit code. No file argument.

```bash
pogo doctor
```

What the report means: [agents.md](agents.md).

## skill

`pogo skill [--list | --framework NAME]`

Print the agent skill on stdout. `--list` prints framework ids, one per line. `--framework NAME` prints one section. The two flags cannot be combined. No file argument. Extra words are ignored.

```bash
pogo skill
pogo skill --list
pogo skill --framework sveltekit
```

## help

`pogo help [command]`

`pogo help`, `pogo --help`, and `pogo -h` print the main page and exit 0. `pogo help run`, `pogo run --help`, and `pogo run -h` print that command. One command name. `docker` is a guide, not a container runtime.

```bash
pogo help
pogo help run
pogo help docker
```

## version

`pogo version`

Print `pogo` and the running version (this tree: `pogo 1.0.5`). Same as `pogo -V` and `pogo --version` with no command. Extra words are ignored.

```bash
pogo version
pogo --version
```

## docker

`pogo docker`

Print the Docker guide and exit 0. Does not start a container. Same text as `pogo help docker` and `pogo docker --help`. No arguments. To start Docker from the host: `pogo run -- docker ...`.

The written guide: [docker.md](docker.md).
