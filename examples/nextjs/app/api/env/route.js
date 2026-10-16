import { createHash } from "node:crypto";
import { NextResponse } from "next/server";

export const dynamic = "force-dynamic";

const KEYS = [
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

export function GET() {
  const missing = KEYS.filter((key) => !process.env[key]);
  if (missing.length) {
    return NextResponse.json({ ok: false, missing }, { status: 500 });
  }
  const vars = {};
  for (const key of KEYS) {
    const value = process.env[key];
    vars[key] = {
      present: true,
      length: value.length,
      sha256: createHash("sha256").update(value).digest("hex"),
      masked: mask(value),
    };
  }
  return NextResponse.json({ ok: true, vars });
}
