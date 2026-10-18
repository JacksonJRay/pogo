# Shared environment demo

One encrypted file can supply Flask and SvelteKit on the host and Node in Docker.
The `.env.example` values are fake. Pages expose masked values, lengths, and
hashes; use fake fixtures and keep diagnostics restricted to trusted access.

## Prepare

Run these commands from the Pogo repository root. Build Pogo and add it to PATH:

```bash
make build
export PATH="$PWD:$PATH"
cd examples/shared-env-demo
python3 -m venv .venv
.venv/bin/python -m pip install -r python/requirements.txt
(cd web && npm ci)
cp .env.example .env
pogo convert .env
```

Enter a passphrase of at least 15 characters and confirm it. The documented
`pogo-example-passphrase` is a fake test value; choose your own for real data.
Conversion preserves `.env`. Verify the encrypted file works, then remove your
plaintext copy. Keep `.env.example` as the labeled fake fixture.

## Start the host apps

Start each terminal in the repository root.

Python, at http://127.0.0.1:43121:

```bash
cd examples/shared-env-demo/python
pogo run -f ../.env.enc -- ../.venv/bin/python app.py
```

SvelteKit, at http://127.0.0.1:43122:

```bash
cd examples/shared-env-demo/web
pogo run -f ../.env.enc -- npm run dev
```

SvelteKit reads `$env/dynamic/private` on the server. Its production commands are:

```bash
npm run build
PORT=43122 HOST=127.0.0.1 pogo run -f ../.env.enc -- npm start
```

Stop the development server before using the same port for production. Flask's
built-in server is for development.

## Start Node in Docker

From the repository root:

```bash
cd examples/shared-env-demo
read -rs POGO_PASS && printf '%s' "$POGO_PASS" | docker compose run -T --service-ports --rm --build node ; unset POGO_PASS
```

The entrypoint runs Pogo with `--key-stdin`; the passphrase is consumed before
Node starts. There is no passphrase file. The Compose mapping publishes port
43123 on host interfaces by default. Restrict the mapping to
`127.0.0.1:43123:43123` before using it as a local-only diagnostic service, or
apply access controls. A localhost URL describes where to browse, not the
network exposure of the default mapping.

Pogo uses exec, so Node becomes PID 1 and receives container stop signals.
Stop host apps with Ctrl-C and the container with Ctrl-C or `docker compose down`.

## Verify

Open each page and its `/env` endpoint. Matching variable hashes and lengths
show that the same bytes reached each app. Partial values and unsalted hashes
still disclose information; remove or protect diagnostics before using real
credentials.

The local workspace may include `test_demo.py` as an optional integration
runner. When that file is present and dependencies are installed, run
`.venv/bin/python test_demo.py` from this directory. It uses disposable fake
fixtures and temporary ports.

## Troubleshooting

From `python/` or `web/`, pass `-f ../.env.enc`. Use an absolute path in services.
From this directory, a missing default file produces:

```text
No .env.enc found in this folder.
Create one with:
  pogo init
Or convert an existing file with:
  pogo convert .env
```

A wrong passphrase or damaged file returns exit 2. Noninteractive startup requires
`--key-stdin` or `--key-env`; interactive Docker startup requires a terminal.
Run `pogo help docker` for the supported recipes. The same guide is in [docs/docker.md](../../docs/docker.md). If the container build cannot
find Pogo, run `make build` from the repository root first.
