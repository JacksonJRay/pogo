package passphrase

import (
	"regexp"
	"strings"
	"testing"
)

// ovisPrompt is the set of lines Ovis 0.5.x treats as a secret prompt
// (shared/secret-prompt.ts, the default set). A longer sentence on the same
// line, such as "Enter passphrase (min 15 characters):", does not match.
var ovisPrompt = regexp.MustCompile(`(?i)^(?:enter passphrase(?: for [^\r\n]+)?|enter (?:old |new |same )?passphrase(?: again)?(?: \(empty for no passphrase\))?):$`)

func TestOvisLinesMatch(t *testing.T) {
	cases := []struct {
		prompt string
		file   string
		kind   PromptKind
		last   string
	}{
		{prompt: "Enter passphrase: ", file: ".env.enc", kind: KindCurrent, last: "Enter passphrase for key '.env.enc':"},
		{prompt: "Enter passphrase: ", file: "", kind: KindCurrent, last: "Enter passphrase:"},
		{prompt: "Current passphrase for .env.enc: ", file: ".env.enc", kind: KindCurrent, last: "Enter old passphrase:"},
		{prompt: "New passphrase for .env.enc: ", file: ".env.enc", kind: KindNew, last: "Enter new passphrase:"},
		{prompt: "Confirm new passphrase for .env.enc: ", file: ".env.enc", kind: KindConfirm, last: "Enter same passphrase again:"},
	}
	for _, tc := range cases {
		shown := ovisTerminalPrompt(tc.prompt, tc.file, tc.kind)
		last := shown
		if i := strings.LastIndex(shown, "\n"); i >= 0 {
			if !strings.Contains(shown[:i], tc.file) && tc.file != "" && tc.kind != KindCurrent {
				t.Errorf("hint missing file %q in %q", tc.file, shown)
			}
			last = shown[i+1:]
		}
		last = strings.TrimRight(last, " \t")
		if last != tc.last {
			t.Errorf("last line %q want %q (from %q)", last, tc.last, shown)
		}
		if !ovisPrompt.MatchString(last) {
			t.Errorf("Ovis would not mask %q", last)
		}
		if strings.Contains(last, "?") {
			t.Errorf("question mark hides the prompt from Ovis: %q", last)
		}
	}
	for _, skipped := range []string{
		"Enter passphrase (min 15 characters):",
		"New passphrase for .env.enc:",
		"Confirm new passphrase for .env.enc:",
		"Enter it again to confirm:",
	} {
		if ovisPrompt.MatchString(skipped) {
			t.Errorf("expected Ovis to ignore %q", skipped)
		}
	}
}

func TestOvisKeyNameRejectsQuotes(t *testing.T) {
	shown := ovisTerminalPrompt("Enter passphrase: ", "a'b.enc", KindCurrent)
	if strings.Contains(shown, "a'b") || !strings.HasPrefix(shown, "Enter passphrase:") {
		t.Fatalf("%q", shown)
	}
}
