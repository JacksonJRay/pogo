# Changelog

## 2026-10-16 — Pogo security maintenance

- Upgraded golang.org/x/crypto to v0.58.0, golang.org/x/sys to v0.49.0, and golang.org/x/term to v0.47.0.
- Selected Go 1.27.2 for patched builds and updated Pogo’s from-source Docker build.
- Retained release symbol tables for accurate executable vulnerability analysis.
- Added `make security`, a Pogo-only CycloneDX SBOM, unfiltered scan evidence, and a CLI usability assessment.
- Recorded zero current CVE identifiers and retained the module-only GO-2026-5932 OpenPGP advisory with zero reported reachable vulnerabilities.
- Added portable builds, reproducible security reports, and concise usage documentation.
