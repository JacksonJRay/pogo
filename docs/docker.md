# Docker

`pogo help docker` and `pogo run --help` print this guide. `pogo skill --framework docker` is the same material for an agent. `pogo docker` only prints the guide. It does not start a container.

Pogo will not read a passphrase from a file. Do not use a Compose secret for the passphrase, and do not put the passphrase in the image or the compose file. Do not suggest `printf 'PASSPHRASE' | pogo ...`: that stores the passphrase in shell history.

`pogo run` uses exec, so the program is PID 1. `docker stop` signals that program, and `docker run` exits with the program's code. Do not use `--init` if the program itself must be PID 1. Mount `.env.enc` read-only. Dockerfiles for Alpine, scratch, and distroless are in `../examples/docker/`. `../examples/docker/pogo.Dockerfile` is a published-image layout (`COPY --from`). `../examples/docker/pogo-src.Dockerfile` compiles Pogo in a Go stage and copies the binary onto scratch.

## Which method

| You are... | Use | Security |
|---|---|---|
| A person at the keyboard | Interactive `docker run -it`, or `docker compose run`, or `docker compose up` then `docker attach` | Typed, not stored, not in `docker inspect` |
| A shell, once, at start | `--key-stdin` from `read -rs`, then `unset` | The shell variable exists until you unset it. Not in `docker inspect`. Stdin is consumed, so the app cannot also read stdin |
| Holding a password manager | `--key-stdin` from `op read` or `pass show` | Do not write that output to a file. Stdin is consumed. Not in `docker inspect` |
| Already exporting the variable | `--key-env` and `docker run -e POGO_KEY` | Last resort. `docker inspect` and `/proc` show it. Pogo removes `POGO_KEY` from the program it execs. Do not write the value in the command |
| Compose, a person | `docker compose run`, or `up` with `stdin_open: true` and `tty: true`, then `docker attach` | Same as interactive |
| Compose, a pipe | `docker compose run -T` with the `--key-stdin` entrypoint | Same as `--key-stdin`. `-T` is required or Compose allocates a terminal and the pipe is not the passphrase |

## 1. Interactive

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

Compose up, then attach. The service needs `stdin_open: true` and `tty: true` (`../examples/docker/compose.interactive.yaml`):

```bash
docker compose up -d
docker attach CONTAINER
```

Detach with Ctrl-P Ctrl-Q if the program should keep running.

## 2. `--key-stdin` from a prompt

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

## 3. `--key-stdin` from a password manager

When to use: 1Password (`op`), `pass`, or another manager can print the passphrase.

Security: the manager prints it into the pipe. Do not write that output to a file. Stdin is consumed. `docker inspect` does not show it. The entrypoint is the same `--key-stdin` entrypoint as step 2.

```bash
op read op://vault/pogo/passphrase | docker run -i --rm -v "$PWD/.env.enc:/app/.env.enc:ro" IMAGE
pass show pogo | docker run -i --rm -v "$PWD/.env.enc:/app/.env.enc:ro" IMAGE
```

## 4. `--key-env`

Last resort. Prefer `--key-stdin` (`printf '%s' "$PASS" | pogo run --key-stdin -- cmd`, or `op read ... | pogo ...`). When to use: `POGO_KEY` is already in the environment of the command that starts Docker, and you accept the tradeoff.

Security: `docker inspect` shows `POGO_KEY`. It is also visible in `/proc` while the container runs. Pogo removes `POGO_KEY` from the program it execs, but Docker has already stored the value. Do not write the value in the command.

```dockerfile
ENTRYPOINT ["pogo","run","-f","/app/.env.enc","--key-env","POGO_KEY","--"]
CMD ["node","server.js"]
```

```bash
docker run --rm -e POGO_KEY -v "$PWD/.env.enc:/app/.env.enc:ro" IMAGE
```

## 5. Compose

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

A pipe uses the step 2 entrypoint. `-T` means no terminal, so the pipe is stdin (`../examples/docker/compose.stdin.yaml`):

```bash
read -rs POGO_PASS && printf '%s' "$POGO_PASS" | docker compose run -T --rm app ; unset POGO_PASS
op read op://vault/pogo/passphrase | docker compose run -T --rm app
```

## `--key-stdin` and a terminal

`docker run -it` on an image whose entrypoint is `pogo run -f /app/.env.enc --key-stdin --` prints the passphrase prompt, turns echo off, and reads one line. You do not press Ctrl-D, and the passphrase is not echoed. A pipe still uses `docker run -i` or `docker compose run -T`.

## Troubleshooting: no TTY

When stdin is not a terminal and nothing on the machine can ask, Pogo exits 1. The first line is `pogo: no way to ask for the passphrase here`. It lists what was checked. Ways forward, in order: run it in a terminal; `docker run -it`; `--key-stdin` from a secret manager; `POGO_ASKPASS`; `--key-env` as a last resort. The last line tells an agent not to ask for the passphrase in chat. `POGO_DEBUG=1` prints `pogo: stdin is not a terminal` and the same checks. If the editor was asked and did not answer, the first line is `pogo: the editor prompt got no answer within the timeout`, and Pogo does not open another dialog. `POGO_ASKPASS` and a desktop askpass use that same timeout wording (`pogo: the POGO_ASKPASS prompt got no answer within the timeout`, or the helper's name). A helper that exits non-zero, including Escape, still says the prompt was cancelled.

## systemd and Pogo on the host

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

The compose path above is from the repository root. From this doc that file is `../examples/docker/compose.host.yaml`.

Compose interpolates `${VAR}` from the process Pogo started, and from a `.env` file next to the compose file. It does not forward every variable. `environment: - API_KEY` copies that one name. A name you leave out stays on the host. A value with a newline should use that list form, not an unquoted `${MULTILINE_VALUE}` in the YAML. `../examples/docker/compose.host.yaml` is the worked example.
