import { createHash } from "node:crypto";
import { env } from "$env/dynamic/private";

export const KEYS = [
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
];

function mask(value) {
  if (value.length <= 4) return "****";
  return value.slice(0, 2) + "…" + value.slice(-2);
}

export function report() {
  const missing = [];
  const vars = {};
  const rows = [];
  for (const key of KEYS) {
    if (env[key] === undefined) {
      missing.push(key);
      vars[key] = { present: false, length: 0, sha256: "", masked: "not set" };
      rows.push({ key, masked: "not set", present: false });
      continue;
    }
    const value = env[key];
    const item = {
      present: true,
      length: value.length,
      sha256: createHash("sha256").update(value).digest("hex"),
      masked: mask(value),
    };
    vars[key] = item;
    rows.push({ key, masked: item.masked, present: true });
  }
  return { ok: missing.length === 0, app: "sveltekit", missing, vars, rows };
}
