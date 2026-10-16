#!/usr/bin/env python3
import hashlib
import os

from flask import Flask, jsonify

KEYS = [
    "PORT",
    "HOST",
    "DATABASE_URL",
    "API_KEY",
    "JWT_SECRET",
    "QUOTED_VALUE",
    "SPACED_VALUE",
    "EQUALS_VALUE",
    "MULTILINE_VALUE",
    "VITE_PUBLIC_TITLE",
    "NEXT_PUBLIC_TITLE",
]

app = Flask(__name__)


def mask(value: str) -> str:
    if len(value) <= 4:
        return "****"
    return value[:2] + "…" + value[-2:]


def report():
    missing = [key for key in KEYS if not os.environ.get(key)]
    variables = {}
    for key in KEYS:
        value = os.environ.get(key, "")
        variables[key] = {
            "present": key not in missing,
            "length": len(value),
            "sha256": hashlib.sha256(value.encode()).hexdigest(),
            "masked": mask(value),
        }
    return {"ok": not missing, "missing": missing, "vars": variables}


@app.get("/env")
def env():
    body = report()
    return jsonify(body), (200 if body["ok"] else 500)


@app.get("/")
def index():
    return "<p>Pogo Flask example. See <a href='/env'>/env</a>.</p>"


if __name__ == "__main__":
    body = report()
    if not body["ok"]:
        raise SystemExit("missing required env: " + ", ".join(body["missing"]))
    app.run(host="127.0.0.1", port=int(os.environ["PORT"]))
