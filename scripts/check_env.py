#!/usr/bin/env python3
"""Check that a process received Pogo's decrypted example variables.

Values are obviously fake and exist only so tests can prove a round trip.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
import urllib.request

# Parsed values. The dotenv writer below is the file Pogo encrypts.
EXPECTED = {
    "PORT": "43111",
    "HOST": "127.0.0.1",
    "DATABASE_URL": "postgres://pogo_example:not-a-secret@127.0.0.1:5432/pogo_demo",
    "API_KEY": "pogo_test_api_key_example",
    "JWT_SECRET": "pogo_test_jwt_secret_example",
    "QUOTED_VALUE": 'say "hello" world',
    "SPACED_VALUE": "hello world",
    "EQUALS_VALUE": "a=b=c",
    "MULTILINE_VALUE": "line one\nline two",
    "VITE_PUBLIC_TITLE": "pogo-vite-public-title",
    "NEXT_PUBLIC_TITLE": "pogo-next-public-title",
}

PRODUCTION_API_KEY = "pogo_test_api_key_production"

DEFAULT_KEYS = list(EXPECTED)


def sha256(value: str) -> str:
    return hashlib.sha256(value.encode()).hexdigest()


def dotenv_text() -> str:
    return (
        "# pogo example env — fake values, not secrets\n"
        "\n"
        f"PORT={EXPECTED['PORT']}\n"
        f"HOST={EXPECTED['HOST']}\n"
        f"DATABASE_URL={EXPECTED['DATABASE_URL']}\n"
        f"export API_KEY={EXPECTED['API_KEY']}\n"
        f"JWT_SECRET={EXPECTED['JWT_SECRET']}\n"
        'QUOTED_VALUE="say \\"hello\\" world"\n'
        'SPACED_VALUE="hello world"\n'
        f"EQUALS_VALUE={EXPECTED['EQUALS_VALUE']}\n"
        'MULTILINE_VALUE="line one\nline two"\n'
        f"VITE_PUBLIC_TITLE={EXPECTED['VITE_PUBLIC_TITLE']}\n"
        f"NEXT_PUBLIC_TITLE={EXPECTED['NEXT_PUBLIC_TITLE']}\n"
        "\n"
        "[production]\n"
        f"API_KEY={PRODUCTION_API_KEY}\n"
    )


def mask(value: str) -> str:
    if len(value) <= 4:
        return "****"
    return value[:2] + "…" + value[-2:]


def apply_overrides(overrides: list[str]) -> dict[str, str]:
    expected = dict(EXPECTED)
    for item in overrides:
        key, sep, val = item.partition("=")
        if not sep:
            raise SystemExit(f"--override needs KEY=VALUE, got {item!r}")
        expected[key] = val.replace("\\n", "\n")
    return expected


def check_report(report: dict, keys: list[str], expected: dict[str, str]) -> list[str]:
    problems = []
    vars_ = report.get("vars") or {}
    for key in keys:
        item = vars_.get(key)
        exp = expected[key]
        if not isinstance(item, dict):
            problems.append(f"{key}: missing from response")
            continue
        if item.get("present") is not True:
            problems.append(f"{key}: not present")
            continue
        if item.get("length") != len(exp):
            problems.append(f"{key}: length {item.get('length')} want {len(exp)}")
        if item.get("sha256") != sha256(exp):
            problems.append(f"{key}: sha256 mismatch")
        if item.get("masked") == exp:
            problems.append(f"{key}: response included the full value")
    return problems


def check_lines(text: str, keys: list[str], expected: dict[str, str]) -> list[str]:
    found: dict[str, str | None] = {}
    for line in text.splitlines():
        if not line.startswith("POGO_ENV\t"):
            continue
        parts = line.split("\t")
        if len(parts) != 3:
            continue
        _, key, payload = parts
        if payload == "MISSING":
            found[key] = None
        else:
            found[key] = bytes.fromhex(payload).decode()
    problems = []
    for key in keys:
        if key not in found or found[key] != expected[key]:
            problems.append(f"{key}: stdout mismatch")
    return problems


def main() -> int:
    parser = argparse.ArgumentParser(description="Check Pogo example env delivery")
    parser.add_argument("--write-dotenv")
    parser.add_argument("--http")
    parser.add_argument("--stdin-lines", action="store_true")
    parser.add_argument("--file")
    parser.add_argument("--keys", default=",".join(DEFAULT_KEYS))
    parser.add_argument("--override", action="append", default=[])
    parser.add_argument("--contains", action="append", default=[], help="file or http body must include this string")
    args = parser.parse_args()
    expected = apply_overrides(args.override)
    keys = [k for k in args.keys.split(",") if k]

    if args.write_dotenv:
        with open(args.write_dotenv, "w", encoding="utf-8") as fh:
            fh.write(dotenv_text())
        return 0

    problems: list[str] = []
    body = ""
    if args.http:
        with urllib.request.urlopen(args.http, timeout=5) as resp:
            body = resp.read().decode()
        report = json.loads(body)
        problems.extend(check_report(report, keys, expected))
    elif args.stdin_lines:
        body = sys.stdin.read()
        problems.extend(check_lines(body, keys, expected))
    elif args.file:
        with open(args.file, encoding="utf-8") as fh:
            body = fh.read()
        if "POGO_ENV\t" in body:
            problems.extend(check_lines(body, keys, expected))
        else:
            report = json.loads(body)
            problems.extend(check_report(report, keys, expected))
    else:
        parser.error("pass --write-dotenv, --http, --file, or --stdin-lines")

    for needle in args.contains:
        if needle not in body:
            problems.append(f"missing text {needle!r}")

    if problems:
        print("\n".join(problems), file=sys.stderr)
        return 1
    print("ok")
    return 0


if __name__ == "__main__":
    sys.exit(main())
