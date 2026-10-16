import { createHash } from "node:crypto";
import { env } from "$env/dynamic/private";

export const KEYS = [
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
];

function mask(value) {
  if (value.length <= 4) return "****";
  return value.slice(0, 2) + "…" + value.slice(-2);
}

export function report() {
  const missing = KEYS.filter((key) => env[key] === undefined || env[key] === "");
  const vars = {};
  for (const key of KEYS) {
    const value = env[key] ?? "";
    vars[key] = {
      present: !missing.includes(key),
      length: value.length,
      sha256: createHash("sha256").update(value).digest("hex"),
      masked: mask(value),
    };
  }
  return { ok: missing.length === 0, missing, vars };
}
