package skill

import (
	"bytes"
	"testing"
)

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
