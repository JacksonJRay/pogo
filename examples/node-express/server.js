import { createHash } from "node:crypto";
import express from "express";

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

const missing = KEYS.filter((key) => !process.env[key]);
if (missing.length) {
  console.error("missing required env: " + missing.join(", "));
  process.exit(1);
}

function mask(value) {
  if (value.length <= 4) return "****";
  return value.slice(0, 2) + "…" + value.slice(-2);
}

function report() {
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
  return { ok: true, vars };
}

const app = express();
app.get("/", (_req, res) => {
  res.type("html").send(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Pogo Express example</title>
  <style>
    body { margin: 0; font: 16px/1.5 "Iowan Old Style", Palatino, serif; background: #f4f1ea; color: #1c1915; }
    main { max-width: 40rem; margin: 0 auto; padding: 2.5rem 1.25rem; }
    h1 { font-size: 2rem; margin: 0 0 0.25rem; }
    .stamp { letter-spacing: 0.14em; text-transform: uppercase; font: 12px ui-monospace, monospace; color: #0f6b4c; }
    ul { padding: 0; list-style: none; }
    li { display: flex; justify-content: space-between; gap: 1rem; border-top: 1px solid #ddd6c8; padding: 0.4rem 0; font-family: ui-monospace, monospace; font-size: 14px; }
  </style>
</head>
<body>
  <main>
    <p class="stamp">Pogo · Express</p>
    <h1>Secrets are loaded.</h1>
    <p>Values below are masked. The <a href="/env">/env</a> JSON has lengths and hashes, not the secrets.</p>
    <ul id="vars"></ul>
  </main>
  <script>
    fetch("/env").then((r) => r.json()).then((body) => {
      const ul = document.querySelector("#vars");
      for (const [key, item] of Object.entries(body.vars)) {
        const li = document.createElement("li");
        li.textContent = key + "  " + item.masked;
        ul.appendChild(li);
      }
    });
  </script>
</body>
</html>`);
});
app.get("/env", (_req, res) => {
  res.json(report());
});

const port = process.env.PORT;
app.listen(Number(port), "127.0.0.1", () => {
  console.error("listening on 127.0.0.1:" + port);
});
