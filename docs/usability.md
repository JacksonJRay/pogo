# Using Pogo

Build Pogo with `make build`, then add the repository directory to `PATH`.

```bash
export PATH="$PWD:$PATH"
pogo convert .env
pogo run -- your-command
```

`convert` prompts for a passphrase and confirmation. It preserves the source
`.env`; verify that the encrypted file works, then remove your plaintext copy.
Use `pogo edit` to update values and `pogo rekey` to change the passphrase.

## Choosing a startup method

- At a terminal, use the interactive prompt. It waits until you type or press Ctrl-C. The child retains access to stdin. `POGO_TTY_TIMEOUT` is optional and off by default.
- When stdin is not a terminal, Pogo uses `POGO_ASKPASS`, the editor prompt, or a desktop askpass if one of those is already there. A local display is used only when its socket exists. `pogo doctor` reports which. An agent does not collect the passphrase. `POGO_PROMPT_TIMEOUT` bounds the out-of-band wait. An editor that accepts the request and does not answer does not fall through to another dialog.
- For unattended startup, pipe from a configured password manager with
  `--key-stdin`. A pipe consumes stdin and trims whitespace from its ends.
  If stdin is a terminal, `--key-stdin` prompts on stderr, turns echo off, and
  reads one line.
- `--key-env NAME` is a last resort. It reads a process environment variable
  and removes it from the child, but the value stays visible to environment
  readers (`/proc`, `docker inspect`) while Pogo runs. Prefer `--key-stdin`.

Use an absolute `-f` path in services. `--env NAME` selects an environment section;
`--no-override` preserves variables already set in the process environment.

## Recovering from common errors

Run `pogo help` or `pogo help run` for command examples. Missing-file errors show
creation commands. Exit 2 means decryption failed. Exit 4 on create means the
passphrase is shorter than 15 characters, or empty or only whitespace.
`--allow-short-passphrase` skips the 15-character minimum when you create a
passphrase and still rejects a blank passphrase. Opening an existing file
rejects only an empty passphrase with exit 4. After startup, the
child program supplies the exit code.

## Secret handling

Keep passphrases out of command history, files, and logs. Environment-dumping
commands and shell tracing can expose values. Editing uses restricted temporary
plaintext; handled cleanup cannot cover SIGKILL, power loss, physical storage
erasure, or editor-created copies. Configure editor backup, swap, and undo
settings deliberately.

Frameworks can bind variables at build time. Use server-side runtime environment
reads when values must be supplied at startup, and validate the integration with
the versions used by your project.
