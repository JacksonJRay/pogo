#!/usr/bin/env python3
import hashlib
import os

from fastapi import FastAPI
from fastapi.responses import JSONResponse

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

app = FastAPI()


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
    return JSONResponse(body, status_code=200 if body["ok"] else 500)


@app.get("/")
def index():
    return {"ok": True, "hint": "see /env"}
