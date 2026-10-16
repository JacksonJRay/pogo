
## Framework notes

These are integration recipes. Validate them with the framework versions and tools installed in your project.

Each section is one toolchain. Add a new one by appending a `## framework <id>` heading. `pogo skill --list` prints the ids. `pogo skill --framework <id>` prints one section.

The pattern is always the same: Pogo decrypts in memory and execs your command. The command and every process it starts see normal environment variables. Pogo does not import a library into your app.

```bash
printf '%s' "$POGO_PASS" | pogo run --key-stdin -- <command>
```


## framework express

Node.js and Express. The process environment is `process.env` as soon as the process starts.

```bash
pogo run -- npm start
```

`npm start` is usually a shell script that starts `node`. The node process is a grandchild and still inherits the variables. Fail fast in the app if a required variable is missing. Do not print the values.


## framework node

Plain `node`, `npx`, `npm`, `pnpm`, and `yarn` all work. Pogo looks up the program on `PATH` and execs it. Shebangs work because the operating system handles them after exec.

```bash
pogo run -- node print-env.js
pogo run -- npm run start
pogo run -- pnpm run start
pogo run -- yarn run start
pogo run -- npx --no-install node print-env.js
```


## framework npm-dev

`npm run dev` that spawns another Node process. The child inherits the environment. You do not re-wrap the child in Pogo.

```bash
pogo run -- npm run dev
```


## framework nodemon

nodemon is a watcher. It restarts `node` on file changes. Start nodemon under Pogo once. Restarts keep the environment nodemon was given.

```bash
pogo run -- npx nodemon server.js
```

Ctrl-C goes to the process group because Pogo has exec'd nodemon; nodemon is the foreground program.


## framework nextjs

Next.js dev server. Server code reads `process.env` at runtime. The browser only sees names that start with `NEXT_PUBLIC_`, and those are copied into the client bundle when Next compiles it (when `next dev` starts, or during `next build`).

```bash
pogo run -- npm run dev
```

A private key such as `API_KEY` is available in route handlers and server components. It is not automatically sent to the browser. `NEXT_PUBLIC_TITLE` is visible to client code only because of the prefix, and only if it was set when the client bundle was built.


## framework vite

Vite dev server with React. Client code sees `import.meta.env.VITE_*` only. Vite reads those when the dev server starts and replaces them in the transformed modules. A variable without the `VITE_` prefix is not in the client bundle. Server-side Vite hooks and your own middleware can still read `process.env`.

```bash
pogo run -- npm run dev
```


## framework sveltekit

SvelteKit has two different env APIs:

- `$env/dynamic/private` reads `process.env` when the server runs. Use this with Pogo. It works for `vite dev` and for an adapter-node production server.
- `$env/static/private` is copied into the server bundle at `vite build`. If the variable was unset during the build, the build fails or the value is empty. Starting the built server later with Pogo does not fill static variables.

Dev:

```bash
pogo run -- npm run dev
```

Production (adapter-node writes `build/index.js`; `node build` runs it):

```bash
npm run build
pogo run -- node build
```

Build with ordinary `npm run build` (no secrets). Run the output with Pogo so dynamic private env is present. Set `HOST=127.0.0.1` and `PORT` in the encrypted file if you want the production server on localhost. adapter-node reads `PORT` and `HOST` itself.

Do not put secrets in `$env/static/public` or any `PUBLIC_` prefix. Those are shipped to the browser.


## framework go

A Go server reads `os.Getenv` / `os.LookupEnv`. `go run` and a compiled binary work the same way. `air` is a watcher that rebuilds and restarts `go run`; start `air` under Pogo so the child `go` process inherits the variables.

```bash
pogo run -- go run .
pogo run -- ./server
pogo run -- air
```

Ctrl-C stops the foreground program (the `go` tool or your binary) because of exec.


## framework flask

Flask (and any WSGI app) reads `os.environ` when the process starts.

```bash
pogo run -- python app.py
```


## framework fastapi

FastAPI under uvicorn. Start uvicorn under Pogo, not the other way around.

```bash
pogo run -- python -m uvicorn app:app --host 127.0.0.1 --port 43111
```

The port flag can match `PORT` from the file, or the app can read `PORT` itself.


## framework django

Django's `runserver` reads `os.environ` in settings and views.

```bash
pogo run -- python manage.py runserver 127.0.0.1:43111
```


## framework ruby

A Ruby script reads `ENV`.

```bash
pogo run -- ruby show.rb
pogo run -- ./show.rb
```


## framework rust

`cargo run` compiles if needed and execs the binary. The binary reads `std::env::var`. No Rust library is required.

```bash
pogo run -- cargo run
```


## framework shell

A script with a shebang works. Pogo execs the script; the kernel runs the interpreter and passes the environment through.

```bash
pogo run -- ./show.sh
```

Quote expansions in the script (`"$QUOTED_VALUE"`) so the shell does not split on spaces. The value is already in the environment; Pogo does not pass it through a shell itself.


## framework docker

`pogo help docker` is the step-by-step guide. Pogo will not read a passphrase from a file. Do not use a Compose secret for the passphrase.

`pogo run` uses exec, so inside a container the program is PID 1. `docker stop` signals that program. The container's exit code is the program's exit code. Do not use `--init` if the program itself must be PID 1.

Mount `.env.enc` read-only. Pick one passphrase method.

Which method should I pick?

| You are... | Use | Security |
|---|---|---|
| A person at the keyboard | Interactive `-it` | Typed, not stored, not in `docker inspect` |
| A shell, once | `--key-stdin` from `read -rs` | Unset the variable after. Stdin is consumed. Not in `docker inspect` |
| Holding a password manager | `--key-stdin` from `op` or `pass` | Do not write the output to a file. Stdin is consumed |
| Already exporting the variable | `--key-env` and `docker run -e POGO_KEY` | Visible in `docker inspect` and `/proc`. Pogo removes it from the child |

### 1. Interactive

When to use: a person can type. Security: nothing is stored; `docker inspect` does not show the passphrase.

```dockerfile
ENTRYPOINT ["pogo","run","-f","/app/.env.enc","--"]
CMD ["node","server.js"]
```

```bash
docker run -it --rm -v "$PWD/.env.enc:/app/.env.enc:ro" IMAGE
```

The prompt is `Enter passphrase (min 15 characters): ` and nothing is echoed.

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

When to use: type it once in the shell. Security: unset the variable; it is not in `docker inspect`. Stdin is consumed, so the app cannot also read stdin.

```dockerfile
ENTRYPOINT ["pogo","run","-f","/app/.env.enc","--key-stdin","--"]
CMD ["node","server.js"]
```

```bash
read -rs POGO_PASS && printf '%s' "$POGO_PASS" | docker run -i --rm -v "$PWD/.env.enc:/app/.env.enc:ro" IMAGE ; unset POGO_PASS
```

### 3. `--key-stdin` from a password manager

When to use: `op` or `pass` can print the passphrase. Security: do not write that output to a file. Same entrypoint as step 2. Stdin is consumed. `docker inspect` does not show it.

```bash
op read op://vault/pogo/passphrase | docker run -i --rm -v "$PWD/.env.enc:/app/.env.enc:ro" IMAGE
pass show pogo | docker run -i --rm -v "$PWD/.env.enc:/app/.env.enc:ro" IMAGE
```

### 4. `--key-env`

When to use: `POGO_KEY` is already in the caller's environment. Security: `docker inspect` shows it, and so does `/proc`. Pogo removes `POGO_KEY` from the program it execs. Do not write the value in the command.

```dockerfile
ENTRYPOINT ["pogo","run","-f","/app/.env.enc","--key-env","POGO_KEY","--"]
CMD ["node","server.js"]
```

```bash
docker run --rm -e POGO_KEY -v "$PWD/.env.enc:/app/.env.enc:ro" IMAGE
```

### 5. Compose

When to use: a compose file, including `examples/shared-env-demo`. Security: same as the method you pick. No Compose secret file.

Interactive `docker compose run` uses the step 1 entrypoint and shows the same prompt.

`docker compose up -d` plus `docker attach` needs `stdin_open: true` and `tty: true`.

A pipe uses the step 2 entrypoint. `-T` disables the terminal so the pipe is stdin:

```bash
read -rs POGO_PASS && printf '%s' "$POGO_PASS" | docker compose run -T --rm app ; unset POGO_PASS
op read op://vault/pogo/passphrase | docker compose run -T --rm app
```

When stdin is not a terminal and you did not pass `--key-stdin` or `--key-env`, Pogo exits 1 and prints:

```text
no passphrase was provided, and stdin is not a terminal so Pogo cannot ask for one.
If this is Docker, add -it (docker run -it), or use docker compose run, or docker compose up with stdin_open: true and tty: true and then docker attach.
Or pass the passphrase with --key-stdin (docker run -i, or docker compose run -T; the program cannot also read stdin) or --key-env NAME.
Pogo does not read a passphrase from a file.
```

systemd has no terminal. Pipe from a password manager, or use `--key-env` for that process only (visible in `/proc`). Do not point Pogo at a file.

```bash
ExecStart=/bin/sh -c 'op read op://vault/pogo/passphrase | /usr/local/bin/pogo run -f /etc/myapp/.env.enc --key-stdin -- /usr/bin/myapp'
```

### Pogo on the host

The Docker client does not copy every variable into the container. Forward each name with `-e NAME`. `sudo` drops the environment unless you pass `-E`. Export `POGO_KEY` first; do not put the passphrase in the command.

```bash
pogo run --key-env POGO_KEY -- docker run --rm -e API_KEY alpine printenv API_KEY
pogo run --key-env POGO_KEY -- sudo -E docker run --rm -e API_KEY alpine printenv API_KEY
```

`docker compose` interpolates `${VAR}` from the Compose process and does not forward the whole environment. `environment: - API_KEY` copies that one name. A name you leave out stays on the host. A newline should use the list form (`- MULTILINE_VALUE`). See `examples/docker/compose.host.yaml`.

```bash
pogo run --key-env POGO_KEY -- sudo -E docker compose -f examples/docker/compose.host.yaml up --abort-on-container-exit --exit-code-from passed
```

The shared demo uses one `.env.enc` for Flask and SvelteKit on the host (`pogo run`) and Node in Docker (`--key-stdin`, file mounted read-only, passphrase piped, no key file).


## framework watchers

Process managers and file watchers (nodemon, air, `npm run dev`, Django's reloader, Vite) should be the program after `--`. They inherit the environment and pass it to the processes they spawn. You do not wrap the child again.

Signals: after exec, Ctrl-C is delivered to that program's foreground process group. Pogo is no longer running, so it does not need a signal forwarder. A TTY stays a TTY, which interactive dev servers require.
