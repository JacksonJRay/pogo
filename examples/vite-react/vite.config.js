import react from "@vitejs/plugin-react";
import { createHash } from "node:crypto";
import { defineConfig } from "vite";

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
  if (!value || value.length <= 4) return "****";
  return value.slice(0, 2) + "…" + value.slice(-2);
}

function report(env) {
  const missing = KEYS.filter((key) => !env[key]);
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

function envProbe() {
  return {
    name: "pogo-env-probe",
    configureServer(server) {
      server.middlewares.use("/env", (_req, res) => {
        const body = report(process.env);
        res.statusCode = body.ok ? 200 : 500;
        res.setHeader("content-type", "application/json");
        res.end(JSON.stringify(body));
      });
    },
  };
}

export default defineConfig({
  plugins: [react(), envProbe()],
  server: {
    host: "127.0.0.1",
    port: Number(process.env.PORT) || 43111,
    strictPort: true,
  },
});
