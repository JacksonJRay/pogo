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

## 2026-10-17 — 1.0.4 rescan

Release binaries were built with Go 1.27.2 (`go version -m` on `pogo-linux-amd64`
reports `go1.27.2`), `CGO_ENABLED=0`, `-trimpath`, `-buildvcs=false`, and `-ldflags=-w`.
Symbol tables are still present.

`govulncheck` v1.9.0 (`./...` and `-mode=binary -show verbose` on `pogo-linux-amd64`)
scanned the go1.27.2 standard library and the modules below. Symbol and package
results are empty. The only module result is `GO-2026-5932`. A later rescan for the
Mozilla fallback roots added `golang.org/x/crypto/x509roots/fallback`
`v0.0.0-20261016151342-0ae6809b7559`. That module had no vulnerability ids.
`GO-2026-5932` is still the only module finding and is still not reachable.

| ID | Package | Version | Reachable | Resolution |
|---|---|---|---|---|
| GO-2026-5932 | golang.org/x/crypto | v0.58.0 | No. Pogo does not import `golang.org/x/crypto/openpgp`. Source and binary govulncheck report no affected symbols or imported packages. | No fixed version (introduced at 0, fixed in N/A). Keep Argon2id and BLAKE2b. Do not switch the module to clear an unused OpenPGP package. |

OSV query of Go stdlib 1.27.2, toolchain 1.27.2, `golang.org/x/sys` v0.49.0, and
`golang.org/x/term` v0.47.0 returned no vulnerability ids. `osv-scanner` `scan source -r .`
reported the same `GO-2026-5932` entry from `security/pogo.cdx.json`.

The same source scan also reported npm advisories in example lockfiles. Those
packages are not linked into the `pogo` binary and were not changed for 1.0.4.
`braces` 3.0.3 (`GHSA-vfj7-8cjw-p6xm`, `examples/node-express/package-lock.json`)
has no fixed version in that report. The others have a fixed version upstream:

- `next` 14.2.35 in `examples/nextjs/package-lock.json`: GHSA-2xp9-vwfh-vxw4, GHSA-36qx-fr4f-26g5, GHSA-3g8h-86w9-wvmq, GHSA-3x4c-7xq6-9pq8, GHSA-4633-3j49-mh5q, GHSA-4c39-4ccg-62r3, GHSA-68g3-v927-f742, GHSA-89xv-2m56-2m9x, GHSA-8h8q-6873-q5fj, GHSA-955p-x3mx-jcvp, GHSA-9g9p-9gw9-jx7f, GHSA-c4j6-fc7j-m34r, GHSA-ffhc-5mcf-pf4q, GHSA-ggv3-7p47-pfv8, GHSA-gx5p-jg67-6x7h, GHSA-h25m-26qc-wcjf, GHSA-h64f-5h5j-jqjh, GHSA-m99w-x7hq-7vfj, GHSA-p293-qw3h-jr36, GHSA-p9j2-gv94-2wf4, GHSA-q4gf-8mx6-v5v3, GHSA-vfv6-92ff-j949, GHSA-wfc6-r584-vfw7
- `postcss` 8.4.31 in `examples/nextjs/package-lock.json`: GHSA-6g55-p6wh-862q, GHSA-fxqj-rqcc-2cmp, GHSA-qx2v-qp2m-jg93, GHSA-r28c-9q8g-f849
- `cookie` 0.6.0 (dev) in `examples/sveltekit/package-lock.json` and `examples/shared-env-demo/web/package-lock.json`: GHSA-pxg6-pf52-xh8x

Plan for the example lockfiles: leave them out of the 1.0.4 CLI release. They are
demo apps, not the static binary.

Resolved modules are `golang.org/x/crypto v0.58.0`, `golang.org/x/sys v0.49.0`,
`golang.org/x/term v0.47.0`, and `golang.org/x/crypto/x509roots/fallback`
`v0.0.0-20261016151342-0ae6809b7559` (embedded Mozilla roots, used only when the
host trust store is empty). Dependencies require Go 1.26 or newer; the module
selects Go 1.27.2 for patched builds. The committed CycloneDX file was generated
before that fallback module was added.

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
The threat model and file format are in [docs/security.md](../docs/security.md).
