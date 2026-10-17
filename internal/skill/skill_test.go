package skill

import (
	"bytes"
	"testing"
)

func TestAgentRules(t *testing.T) {
	text := string(Document)
	for _, phrase := range []string{
		"Prefer `--key-stdin` over `--key-env`",
		"op read ... | pogo run --key-stdin -- cmd",
		"pass show",
		"Do not guess, try, or search the filesystem for a passphrase",
		"printf 'PASSPHRASE' | pogo",
		"pogo waits up to 10 minutes",
		"visible in `/proc` and `docker inspect`",
		"only as a last resort",
		"`bash -lc` can reset `PATH`",
		"/usr/local/bin",
		"Never pass `--key-file` or `--new-key-file`",
		"pogo doctor",
		"pogo run -- <command>` with no key flags",
		"does not collect the passphrase",
		"do not ask the person to paste the passphrase into chat",
		"pogo: the editor prompt got no answer within the timeout",
		"pogo: the POGO_ASKPASS prompt got no answer within the timeout",
		"POGO_PROMPT_TIMEOUT",
		"pogo-passphrase-for-.env.enc.invalid",
		"pogo-new-passphrase-for-.env.enc.invalid",
		"pogo-confirm-passphrase-for-.env.enc.invalid",
		"pogo doctor` covers init, edit, run, encrypt, decrypt, convert, and rekey",
		"run `pogo edit` with no key flags, tell the person a tab will open, and wait",
		"pogo upgrade --check",
		"pogo upgrade --yes",
		"POGO_EDITOR",
		"Do not enter a Git password or a personal access token",
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
