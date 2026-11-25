package passphrase

import "strings"

// Ovis (the Herdr terminal UI) masks input when the last line of the pane
// matches the prompts in its shared/secret-prompt.ts. The line has to end
// with a colon, stay under 120 characters, and be one of the ssh/sudo forms
// that file already recognizes. A note about the file or the 15-character
// minimum is printed on the line above, because extra words on the prompt
// line itself are not recognized.

const (
	ovisOpenFallback = "Enter passphrase: "
	ovisOldLine      = "Enter old passphrase: "
	ovisNewLine      = "Enter new passphrase: "
	ovisAgainLine    = "Enter same passphrase again: "
)

// ovisTerminalPrompt is what a terminal and a Herdr pane print. The last
// line is one Ovis recognizes. hint is empty when the prompt line is enough.
func ovisTerminalPrompt(prompt, file string, kind PromptKind) string {
	hint, line := ovisLines(prompt, file, kind)
	if hint == "" {
		return line
	}
	return hint + "\n" + line
}

func ovisLines(prompt, file string, kind PromptKind) (hint, line string) {
	name := ovisKeyName(file)
	switch kind {
	case KindNew:
		return ovisNewHint(name), ovisNewLine
	case KindConfirm:
		return ovisConfirmHint(name), ovisAgainLine
	default:
		low := strings.ToLower(prompt)
		switch {
		case strings.Contains(low, "current passphrase"):
			return ovisCurrentHint(name), ovisOldLine
		case strings.Contains(low, "new passphrase"):
			return ovisNewHint(name), ovisNewLine
		case strings.Contains(low, "confirm"):
			return ovisConfirmHint(name), ovisAgainLine
		}
		if name != "" {
			return "", "Enter passphrase for key '" + name + "': "
		}
		return "", ovisOpenFallback
	}
}

func ovisNewHint(name string) string {
	if name == "" {
		return "New passphrase. At least 15 characters."
	}
	return "New passphrase for " + name + ". At least 15 characters."
}

func ovisConfirmHint(name string) string {
	if name == "" {
		return "Confirm the new passphrase."
	}
	return "Confirm the new passphrase for " + name + "."
}

func ovisCurrentHint(name string) string {
	if name == "" {
		return "Current passphrase."
	}
	return "Current passphrase for " + name + "."
}

// ovisKeyName is a file name that can sit inside Ovis's
// "Enter passphrase for key '...':" form. Quotes and newlines would break
// that line, so those names are left off the prompt.
func ovisKeyName(file string) string {
	file = strings.TrimSpace(file)
	if file == "" || file == "." || file == ".." || len(file) > 80 {
		return ""
	}
	if strings.ContainsAny(file, "\r\n'’") {
		return ""
	}
	for _, r := range file {
		if r < 0x20 || r == 0x7f {
			return ""
		}
	}
	return file
}
