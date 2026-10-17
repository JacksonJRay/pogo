package skill

import (
	"bytes"
	"testing"
)

func TestAgentRules(t *testing.T) {
	text := string(Document)
	for _, phrase := range []string{
		"Prefer `--key-stdin` over `--key-env`",
		"printf '%s' \"$PASS\" | pogo run --key-stdin -- cmd",
		"op read ... | pogo",
		"visible in `/proc` and `docker inspect`",
		"only as a last resort",
		"`bash -lc` can reset `PATH`",
		"/usr/local/bin",
		"Never pass `--key-file` or `--new-key-file`",
		"Do not pass `--allow-short-passphrase` unless the user explicitly asks",
		"printf '%s' \"$SHORT_PASS\" | pogo convert .env --key-stdin --allow-short-passphrase",
	} {
		if !bytes.Contains(Document, []byte(phrase)) {
			t.Errorf("skill rules missing %q", phrase)
		}
	}
	if !bytes.Contains([]byte(text), []byte("## Rules for agents")) {
		t.Fatal("missing rules section")
	}
}

func TestFrameworkIDs(t *testing.T) {
	ids := IDs()
	need := []string{"express", "node", "sveltekit", "go", "flask", "fastapi", "django", "vite", "nextjs", "shell", "rust"}
	have := map[string]bool{}
	for _, id := range ids {
		have[id] = true
	}
	for _, n := range need {
		if !have[n] {
			t.Errorf("missing framework %s in %v", n, ids)
		}
		body, ok := FrameworkByID(n)
		if !ok || !bytes.Contains([]byte(body), []byte("## framework "+n)) {
			t.Errorf("bad section %s", n)
		}
	}
}
