<p align="center"><img src="assets/pogo-logo.png" alt="Pogo" width="160"></p>
<h1 align="center">Pogo</h1>
<p align="center"><strong>Encrypt <code>.env</code> files and load them at startup.</strong></p>
<p align="center"><a href="https://github.com/JacksonJRay/pogo/releases/latest"><img alt="Latest release" src="https://img.shields.io/github/v/release/JacksonJRay/pogo?style=flat-square&color=0891b2&labelColor=0b1220"></a> <a href="LICENSE"><img alt="MIT license" src="https://img.shields.io/badge/license-MIT-0891b2?style=flat-square&labelColor=0b1220"></a> <img alt="Go 1.26 or newer" src="https://img.shields.io/badge/Go-1.26%2B-0891b2?style=flat-square&labelColor=0b1220"> <img alt="Linux and macOS" src="https://img.shields.io/badge/platforms-linux%20%7C%20macos-0891b2?style=flat-square&labelColor=0b1220"></p>
<p align="center"><a href="#install">Install</a> · <a href="#usage">Usage</a> · <a href="docs/reference.md">Docs</a> · <a href="docs/security.md">Security</a></p>

Plaintext `.env` files sit on disk, in repos, and where agents can read them. Pogo turns that file into `.env.enc` and loads it with `pogo run`.

<table><tr><td align="center"><strong>Encrypted at rest.</strong> Only <code>.env.enc</code> is meant to live on disk. No key file. The passphrase is never stored.</td><td align="center"><strong>One binary.</strong> Download and run. No runtime, SDK, or package install.</td><td align="center"><strong>Agent-safe.</strong> Agents can run your app. The passphrase prompt goes to you, not to them.</td></tr></table>

## Install

```bash
curl -fLO https://github.com/JacksonJRay/pogo/releases/latest/download/pogo-linux-amd64
curl -fLO https://github.com/JacksonJRay/pogo/releases/latest/download/SHA256SUMS
grep pogo-linux-amd64 SHA256SUMS | sha256sum -c
chmod +x pogo-linux-amd64 && sudo mv pogo-linux-amd64 /usr/local/bin/pogo
```

Other platforms: swap in linux-arm64, darwin-amd64 or darwin-arm64 (macOS: shasum -a 256 -c).

## Usage

```bash
pogo convert .env
pogo edit
pogo run -- npm start
```

`convert` leaves `.env` and prints the `rm` command.

MIT. [LICENSE](LICENSE).
