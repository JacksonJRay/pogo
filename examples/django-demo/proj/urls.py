import hashlib
import os

from django.http import JsonResponse
from django.urls import path

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


def mask(value: str) -> str:
    if len(value) <= 4:
        return "****"
    return value[:2] + "…" + value[-2:]


def env(request):
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
    body = {"ok": not missing, "missing": missing, "vars": variables}
    return JsonResponse(body, status=200 if body["ok"] else 500)


urlpatterns = [
    path("", env),
    path("env", env),
]
