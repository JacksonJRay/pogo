/**
 * Node app for the shared-env demo. Runs inside Docker with pogo as PID 1.
 * No npm dependencies: Alpine's node is enough.
 */
import { createHash } from "node:crypto";
import { createServer } from "node:http";

const KEYS = [
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

function report() {
  const vars = {};
  const missing = [];
  const rows = [];
  for (const key of KEYS) {
    if (process.env[key] === undefined) {
      missing.push(key);
      vars[key] = { present: false, length: 0, sha256: "", masked: "not set" };
      rows.push(`<li class="missing"><span class="key">${key}</span><span class="val">not set</span></li>`);
      continue;
    }
    const value = process.env[key];
    const item = {
      present: true,
      length: value.length,
      sha256: createHash("sha256").update(value).digest("hex"),
      masked: mask(value),
    };
    vars[key] = item;
    rows.push(
      `<li class="ok"><span class="key">${key}</span><span class="val">${item.masked}</span></li>`,
    );
  }
  const ok = missing.length === 0;
  return {
    ok,
    app: "node",
    missing,
    vars,
    html: `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Pogo shared env · Node</title>
  <style>
    body { margin: 0; background: #f4f0e6; color: #1c1915; font: 17px/1.45 Palatino, "Iowan Old Style", serif; }
    main { max-width: 36rem; margin: 0 auto; padding: 2.5rem 1.25rem 3rem; }
    .stamp { margin: 0; letter-spacing: 0.16em; text-transform: uppercase; font: 12px ui-monospace, monospace; color: #0e6b4f; }
    h1 { font-size: 2.1rem; line-height: 1.15; margin: 0.4rem 0 0.6rem; }
    .status { color: #0e6b4f; font-family: ui-monospace, monospace; font-size: 13px; }
    ul { list-style: none; padding: 0; margin: 1.25rem 0 0; }
    li { display: flex; justify-content: space-between; gap: 1rem; border-top: 1px solid #ddd4c4; padding: 0.45rem 0; font-family: ui-monospace, monospace; font-size: 14px; }
    .missing .val { color: #9a3412; }
    a { color: #0e6b4f; }
  </style>
</head>
<body>
  <main>
    <p class="stamp">Pogo · Node · Docker · same file</p>
    <h1>Node received the shared environment.</h1>
    <p class="status">${ok ? "Every variable from the shared file is set." : "Some variables are missing."}</p>
    <p>This process is PID 1. Pogo decrypted <code>.env.enc</code> and replaced itself with <code>node</code>. Values are masked.</p>
    <ul>
      ${rows.join("\n      ")}
    </ul>
    <p>Machine-readable hashes: <a href="/env">/env</a>.</p>
  </main>
</body>
</html>`,
  };
}

const port = Number(process.env.NODE_PORT || "43123");
const host = process.env.HOST || "0.0.0.0";

const server = createServer((req, res) => {
  const body = report();
  const path = (req.url || "/").split("?")[0];
  if (path === "/env") {
    const payload = JSON.stringify({
      ok: body.ok,
      app: body.app,
      missing: body.missing,
      vars: body.vars,
    });
    res.writeHead(body.ok ? 200 : 500, { "content-type": "application/json; charset=utf-8" });
    res.end(payload);
    return;
  }
  res.writeHead(200, { "content-type": "text/html; charset=utf-8" });
  res.end(body.html);
});

server.listen(port, host, () => {
  console.log(`node shared env listening on ${host}:${port}`);
});
