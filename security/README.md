# Pogo security verification

Assessment date: 2026-10-16. Scope: Pogo CLI, its three Go dependencies, the build
toolchain, and compiled Pogo executables. Demo dependencies and container base
images are outside this assessment.

## Results

The dependency report and CycloneDX software bill of materials (SBOM) contain
zero CVE identifiers. They retain the non-CVE advisory `GO-2026-5932` for the
unmaintained OpenPGP packages inside `golang.org/x/crypto`. Pogo uses Argon2 and
BLAKE2b; source and executable analysis report zero affected imports or reachable
vulnerabilities. The module advisory has no fixed version and remains visible.

Resolved modules are `golang.org/x/crypto v0.58.0`, `golang.org/x/sys v0.49.0`,
and `golang.org/x/term v0.47.0`. Dependencies require Go 1.26 or newer; the module
selects Go 1.27.2 for patched builds.

## Evidence

- `pogo.cdx.json`: dependency inventory, licenses, scanner provenance, and findings.
- `trivy.json`: dependency findings without ignored IDs or severity filters.
- `govulncheck-source.json`: curated source-analysis JSON stream.
- `govulncheck-binary.json`: curated executable-analysis JSON stream.
- `artifact-sha256.txt`: hashes of the build inputs and executable.

The Go reports retain every finding, scanner configuration, progress record,
and inventory. Advisory descriptions without a corresponding finding are
omitted to reduce artifact size. A module-only finding identifies an advisory
considered at dependency level; imported-package and function entries indicate
closer reachability. No finding is suppressed.

Trivy supplies module evidence. Executable coverage comes from govulncheck;
an empty Trivy executable scan does not establish coverage. Scanner names,
versions, database dates, license notices, and upstream advisory text are kept
as public technical provenance.

## Repeat

```bash
make test
make race
make security
make cross
make security-report
```

`make security` fails on reachable source or executable vulnerabilities and
prints module-level findings. `make security-report` requires Python 3 and Trivy
and regenerates the SBOM, curated Go streams, dependency report, and hashes.
Report generation omits local Git identity metadata from Trivy output while
preserving dependency and vulnerability data. The scanner is pinned at
`govulncheck v1.9.0` and consults the current vulnerability database.

Builds use `-trimpath` to remove machine-specific paths and `-buildvcs=false` to
omit local commit identity. Symbol tables are retained for executable analysis;
`-w` removes debug information. These portable binaries identify their resolved
modules and compiler version without binding to a local commit.

Go vet, unit tests, race checks, source and executable security checks, and
cross compilation cover the verification workflow. Runtime tests run on Linux
amd64; cross compilation targets Linux and macOS on amd64 and arm64.

## Limits

These are dated findings against available advisory databases. Re-run checks
when code, dependencies, or database contents change. The SBOM and scans cannot
guarantee absence of undiscovered vulnerabilities or application-logic bugs.
The retained OpenPGP advisory applies to unused packages in the same module;
removing a vetted Argon2 implementation to clear that module warning requires
separate cryptographic review. Usage guidance is in [docs/usability.md](../docs/usability.md).
