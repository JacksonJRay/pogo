#!/usr/bin/env python3
"""Generate scoped dependency and executable evidence with portable provenance."""
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]


def json_stream(text):
    decoder = json.JSONDecoder()
    records = []
    while text.strip():
        text = text.lstrip()
        record, end = decoder.raw_decode(text)
        records.append(record)
        text = text[end:]
    return records


def main():
    output = ROOT / "security"
    output.mkdir(exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="pogo-report-") as folder:
        work = Path(folder)
        for name, fmt in (("pogo.cdx.json", "cyclonedx"), ("trivy.json", "json")):
            subprocess.run([
                "trivy", "fs", "--scanners", "vuln", "--skip-dirs", "examples",
                "--skip-dirs", "security", "--skip-dirs", "dist", "--format", fmt,
                "--output", str(work / name), ".",
            ], cwd=ROOT, check=True)
            report = json.loads((work / name).read_text())
            # Local repository identity is separate from dependency evidence.
            if name == "trivy.json":
                report.pop("Metadata", None)
            (output / name).write_text(json.dumps(report, indent=2) + "\n")
        for name, args in (
            ("govulncheck-source.json", ["./cmd/pogo", "./internal/..."]),
            ("govulncheck-binary.json", ["-mode", "binary", "./pogo"]),
        ):
            result = subprocess.run([
                "go", "run", "golang.org/x/vuln/cmd/govulncheck@v1.9.0",
                "-format", "json", *args,
            ], cwd=ROOT, check=True, capture_output=True, text=True)
            records = json_stream(result.stdout)
            finding_ids = {record["finding"]["osv"] for record in records if "finding" in record}
            records = [record for record in records
                       if "osv" not in record or record["osv"]["id"] in finding_ids]
            (output / name).write_text("\n".join(json.dumps(record, indent=2)
                                                for record in records) + "\n")
    paths = ["go.mod", "go.sum", "Makefile", "examples/docker/pogo-src.Dockerfile", "pogo"]
    (output / "artifact-sha256.txt").write_text("".join(
        hashlib.sha256((ROOT / path).read_bytes()).hexdigest() + "  " + path + "\n"
        for path in paths
    ))


if __name__ == "__main__":
    main()
