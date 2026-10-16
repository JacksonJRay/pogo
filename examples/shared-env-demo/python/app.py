#!/usr/bin/env python3
"""Tiny Flask app for the shared-env demo.

Runs on the host with `pogo run`. The SvelteKit app and the Node
container read the same encrypted file.
"""

import hashlib
import os

from flask import Flask, jsonify

KEYS = [
    "DATABASE_URL",
    "API_KEY",
    "JWT_SECRET",
    "QUOTED_VALUE",
    "SPACED_VALUE",
    "EQUALS_VALUE",
    "MULTILINE_VALUE",
    "PYTHON_PORT",
    "WEB_PORT",
    "NODE_PORT",
]

app = Flask(__name__)


def mask(value: str) -> str:
    if len(value) <= 4:
        return "****"
    return value[:2] + "…" + value[-2:]


def report():
    variables = {}
    missing = []
    for key in KEYS:
        if key not in os.environ:
            missing.append(key)
            variables[key] = {"present": False, "length": 0, "sha256": "", "masked": "not set"}
            continue
        value = os.environ[key]
        variables[key] = {
            "present": True,
            "length": len(value),
            "sha256": hashlib.sha256(value.encode()).hexdigest(),
            "masked": mask(value),
        }
    return {"ok": not missing, "app": "python", "missing": missing, "vars": variables}


def page(body: dict) -> str:
    rows = []
    for key in KEYS:
        item = body["vars"][key]
        state = "ok" if item["present"] else "missing"
        rows.append(
            f'<li class="{state}"><span class="key">{key}</span>'
            f'<span class="val">{item["masked"]}</span></li>'
        )
    status = "Every variable from the shared file is set." if body["ok"] else "Some variables are missing."
    return f"""<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Pogo shared env · Python</title>
  <style>
    body {{ margin: 0; background: #f4f0e6; color: #1c1915; font: 17px/1.45 Palatino, "Iowan Old Style", serif; }}
    main {{ max-width: 36rem; margin: 0 auto; padding: 2.5rem 1.25rem 3rem; }}
    .stamp {{ margin: 0; letter-spacing: 0.16em; text-transform: uppercase; font: 12px ui-monospace, monospace; color: #0e6b4f; }}
    h1 {{ font-size: 2.1rem; line-height: 1.15; margin: 0.4rem 0 0.6rem; }}
    p {{ margin: 0.4rem 0; }}
    .status {{ color: #0e6b4f; font-family: ui-monospace, monospace; font-size: 13px; }}
    ul {{ list-style: none; padding: 0; margin: 1.25rem 0 0; }}
    li {{ display: flex; justify-content: space-between; gap: 1rem; border-top: 1px solid #ddd4c4; padding: 0.45rem 0; font-family: ui-monospace, monospace; font-size: 14px; }}
    .missing .val {{ color: #9a3412; }}
    a {{ color: #0e6b4f; }}
  </style>
</head>
<body>
  <main>
    <p class="stamp">Pogo · Python · same file</p>
    <h1>Flask received the shared environment.</h1>
    <p class="status">{status}</p>
    <p>Values are masked (first two and last two characters). SvelteKit on the host and Node in Docker read the same <code>.env.enc</code>.</p>
    <ul>
      {''.join(rows)}
    </ul>
    <p>Machine-readable hashes: <a href="/env">/env</a>.</p>
  </main>
</body>
</html>"""


@app.get("/")
def index():
    return page(report())


@app.get("/env")
def env():
    body = report()
    return jsonify(body), (200 if body["ok"] else 500)


if __name__ == "__main__":
    port = int(os.environ.get("PYTHON_PORT", "43121"))
    app.run(host="127.0.0.1", port=port)
