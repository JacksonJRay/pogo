# Pogo

Plaintext `.env` files sit on disk, in repos, and where agents can read them. Pogo encrypts them to `.env.enc` and loads them with `pogo run`; no key file, and the passphrase is not stored.

```bash
# https://github.com/JacksonJRay/pogo/releases/latest
grep pogo-linux-amd64 SHA256SUMS | sha256sum -c
chmod +x pogo-linux-amd64 && mv pogo-linux-amd64 /usr/local/bin/pogo
```

```bash
pogo convert .env
pogo edit
pogo run -- npm start
```

[Commands](docs/commands.md) · [Agents](docs/agents.md) · [Reference](docs/reference.md) · [Security](docs/security.md)

MIT. [LICENSE](LICENSE).
