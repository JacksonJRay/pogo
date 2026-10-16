package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

var keys = []string{
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
}

func main() {
	missing := make([]string, 0)
	for _, key := range keys {
		if v, ok := os.LookupEnv(key); !ok || v == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		log.Fatalf("missing required env: %s", join(missing))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", servePage)
	mux.HandleFunc("/env", serveEnv)
	addr := "127.0.0.1:" + os.Getenv("PORT")
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func serveEnv(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(report())
}

func servePage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, page)
}

func report() map[string]any {
	vars := map[string]any{}
	for _, key := range keys {
		value := os.Getenv(key)
		sum := sha256.Sum256([]byte(value))
		vars[key] = map[string]any{
			"present": true,
			"length":  len(value),
			"sha256":  hex.EncodeToString(sum[:]),
			"masked":  mask(value),
		}
	}
	return map[string]any{"ok": true, "vars": vars}
}

func mask(value string) string {
	if len(value) <= 4 {
		return "****"
	}
	return value[:2] + "…" + value[len(value)-2:]
}

func join(parts []string) string {
	out := ""
	for i, part := range parts {
		if i > 0 {
			out += ", "
		}
		out += part
	}
	return out
}

const page = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Pogo Go example</title>
  <style>
    body { margin: 0; font: 16px/1.5 "Iowan Old Style", Palatino, serif; background: #f7f4ee; color: #1b1916; }
    main { max-width: 40rem; margin: 0 auto; padding: 2.5rem 1.25rem; }
    h1 { font-size: 2rem; margin: 0 0 0.25rem; }
    .stamp { letter-spacing: 0.14em; text-transform: uppercase; font: 12px ui-monospace, monospace; color: #8a3b12; }
    ul { padding: 0; list-style: none; }
    li { display: flex; justify-content: space-between; gap: 1rem; border-top: 1px solid #e4dccd; padding: 0.4rem 0; font-family: ui-monospace, monospace; font-size: 14px; }
  </style>
</head>
<body>
  <main>
    <p class="stamp">Pogo · Go</p>
    <h1>The server has its environment.</h1>
    <p>Masked names from <a href="/env">/env</a>. The process was started by <code>pogo run</code>.</p>
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
</html>`
