package passphrase

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// Diagnosis is what `pogo doctor` prints. It does not ask for a passphrase.
type Diagnosis struct {
	Available bool
	// Backend is "terminal", "POGO_ASKPASS", "editor prompt", a helper label, or "none".
	Backend string
	Detail  string
	Checks  []string
}

// Diagnose reports which backend would be used. It does not run a helper and
// it does not POST to the editor, so nothing is shown to the person.
func Diagnose() Diagnosis {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		debug("stdin is a terminal; the echo-off prompt will be used")
		detail := "Pogo will ask on this terminal with echo off and wait until you type or press Ctrl-C."
		if ttyTimeout() > 0 {
			detail = "Pogo will ask on this terminal with echo off and stop after POGO_TTY_TIMEOUT."
		}
		return Diagnosis{
			Available: true,
			Backend:   "terminal",
			Detail:    detail + " An agent does not collect the passphrase.",
		}
	}
	debug("stdin is not a terminal")
	var checks []string
	note := func(msg string) {
		checks = append(checks, msg)
		debug(msg)
	}
	if os.Getenv("TMUX") != "" || os.Getenv("STY") != "" {
		debug("tmux or screen is set; using the environment this process inherited")
	}

	if path, skip := askpassProgram(); path == "" {
		note(skip)
	} else {
		return Diagnosis{
			Available: true,
			Backend:   "POGO_ASKPASS",
			Detail:    "Pogo will run " + pathBase(path) + " and read one line from its stdout. An agent runs `pogo run -- <command>` with no key flags and tells the person to answer that program. Do not collect the passphrase.",
			Checks:    checks,
		}
	}

	if sock, skip := resolveEditorSocket(); sock == "" {
		note(skip)
	} else if err := probeEditorSocket(sock); err != nil {
		if skipped, ok := err.(*editorSkip); ok {
			note(skipped.reason)
		} else {
			note("editor socket connection refused")
		}
	} else {
		debug("editor socket is present")
		return Diagnosis{
			Available: true,
			Backend:   "editor prompt",
			Detail:    "Pogo will ask in the editor, in the box at the top of the window. code-server and VS Code show Pogo's prompt. Cursor titles the box with the file name. An agent runs `pogo run -- <command>` with no key flags and tells the person to watch for that box. Do not collect the passphrase.",
			Checks:    checks,
		}
	}

	if ok, skip := displayDecision(); !ok {
		note(skip)
		return Diagnosis{Backend: "none", Checks: checks}
	}
	helpers := findHelpers()
	if len(helpers) == 0 {
		note("no desktop askpass found")
		return Diagnosis{Backend: "none", Checks: checks}
	}
	var chosen *foundHelper
	for i := range helpers {
		if reason := helperBlock(helpers[i]); reason != "" {
			note(reason)
			continue
		}
		chosen = &helpers[i]
		break
	}
	if chosen == nil {
		return Diagnosis{Backend: "none", Checks: checks}
	}
	h := *chosen
	debug("display is set; desktop helper " + h.label)
	return Diagnosis{
		Available: true,
		Backend:   h.label,
		Detail:    "Pogo will ask with " + h.label + ". An agent runs `pogo run -- <command>` with no key flags and tells the person to watch for that dialog. Do not collect the passphrase.",
		Checks:    checks,
	}
}

// Report is the text `pogo doctor` writes to stdout.
func (d Diagnosis) Report() string {
	if !d.Available {
		return FormatNoPrompt(d.Checks)
	}
	var b []byte
	b = fmt.Appendf(b, "backend: %s\n%s\n", d.Backend, d.Detail)
	if len(d.Checks) > 0 {
		b = append(b, "Checked:\n"...)
		for _, c := range d.Checks {
			b = append(b, "  "...)
			b = append(b, c...)
			b = append(b, '\n')
		}
	}
	return string(b)
}

func pathBase(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}
