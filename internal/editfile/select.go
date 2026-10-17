package editfile

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"pogo/internal/passphrase"
)

// Choice is the editor Pogo will run. Argv does not include the temp path.
type Choice struct {
	Argv     []string
	Source   string
	Announce bool
	Checks   []string
}

// NoEditorError means nothing can open the file for the person to see.
type NoEditorError struct {
	Checks []string
}

// NoEditorSummary is the first line when no editor can be opened.
const NoEditorSummary = "pogo: no editor can open the file here"

// AgentEditLine tells an agent not to collect plaintext in chat.
const AgentEditLine = "If you are an AI agent: do not ask the person to paste values into chat; ask them to run pogo edit themselves. " + passphrase.AgentRestraint

func (e *NoEditorError) Error() string {
	var b strings.Builder
	b.WriteString(NoEditorSummary)
	b.WriteByte('\n')
	if e == nil || len(e.Checks) == 0 {
		b.WriteString("Nothing could open an editor.\n")
	} else {
		b.WriteString("Checked:\n")
		for _, c := range e.Checks {
			b.WriteString("  ")
			b.WriteString(c)
			b.WriteByte('\n')
		}
	}
	b.WriteString("Ways forward, in order:\n")
	b.WriteString("  1. Run `pogo edit` in your own terminal.\n")
	b.WriteString("  2. Set POGO_EDITOR to a command that waits, for example code --wait.\n")
	b.WriteString("  3. In code-server or Cursor, run `pogo edit` from the integrated terminal.\n")
	b.WriteString(AgentEditLine)
	return b.String()
}

// Select chooses an editor for this process.
func Select() (Choice, error) {
	return SelectTTY(term.IsTerminal(int(os.Stdin.Fd())))
}

// SelectTTY chooses an editor. tty is true when a person can use vim or nano.
// Without a terminal, terminal editors are skipped. A script in EDITOR still runs.
func SelectTTY(tty bool) (Choice, error) {
	var checks []string
	note := func(msg string) { checks = append(checks, msg) }

	if cmd := strings.TrimSpace(os.Getenv("POGO_EDITOR")); cmd != "" {
		fields := strings.Fields(cmd)
		if len(fields) > 0 {
			return Choice{Argv: fields, Source: "POGO_EDITOR", Announce: announces(fields), Checks: checks}, nil
		}
	}
	note("POGO_EDITOR not set")

	if !tty {
		if choice, ok := remoteCLI(); ok {
			choice.Checks = checks
			return choice, nil
		}
		if strings.TrimSpace(os.Getenv("VSCODE_IPC_HOOK_CLI")) == "" {
			note("VSCODE_IPC_HOOK_CLI not set")
		} else {
			note("VSCODE_IPC_HOOK_CLI is set, but no code-server, code, cursor, or codium CLI was found")
		}
	}

	if choice, ok, skip := userEditor("EDITOR", os.Getenv("EDITOR"), tty); ok {
		choice.Checks = checks
		return choice, nil
	} else if skip != "" {
		note(skip)
	}
	if choice, ok, skip := userEditor("VISUAL", os.Getenv("VISUAL"), tty); ok {
		choice.Checks = checks
		return choice, nil
	} else if skip != "" {
		note(skip)
	}

	if tty {
		return Choice{Argv: []string{"vim"}, Source: "vim", Checks: checks}, nil
	}

	if ok, why := passphrase.DisplayReady(); !ok {
		note(why)
	} else if choice, ok := desktopGUI(); ok {
		choice.Checks = checks
		return choice, nil
	} else {
		note("no waiting GUI editor on PATH")
	}
	return Choice{}, &NoEditorError{Checks: checks}
}

// EditorReport is the editor section `pogo doctor` prints.
func EditorReport() string {
	choice, err := Select()
	if err != nil {
		var ne *NoEditorError
		if errors.As(err, &ne) {
			return ne.Error() + "\n"
		}
		return "editor: none\n" + err.Error() + "\n"
	}
	gui := choice.Announce && !term.IsTerminal(int(os.Stdin.Fd()))
	wait := EditorWait(gui)
	if wait > 0 {
		return fmt.Sprintf("editor: %s\nPogo will run this for pogo edit and pogo init and wait up to %s (Ctrl+C to stop). Source: %s.\n", strings.Join(choice.Argv, " "), FormatWait(wait), choice.Source)
	}
	return fmt.Sprintf("editor: %s\nPogo will run this for pogo edit and pogo init and wait until it exits. Source: %s.\n", strings.Join(choice.Argv, " "), choice.Source)
}

func userEditor(name, value string, tty bool) (Choice, bool, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Choice{}, false, name + " not set"
	}
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return Choice{}, false, name + " not set"
	}
	if !tty {
		if reason := skipWithoutTTY(fields); reason != "" {
			return Choice{}, false, name + " is " + reason
		}
	}
	return Choice{Argv: fields, Source: name, Announce: announces(fields)}, true, ""
}

func remoteCLI() (Choice, bool) {
	if strings.TrimSpace(os.Getenv("VSCODE_IPC_HOOK_CLI")) == "" {
		return Choice{}, false
	}
	names := preferredCLINames()
	if node := strings.TrimSpace(os.Getenv("VSCODE_GIT_ASKPASS_NODE")); node != "" {
		parent := filepath.Dir(node)
		dirs := []string{
			filepath.Join(parent, "bin", "remote-cli"),
			filepath.Join(filepath.Dir(parent), "bin", "remote-cli"),
		}
		for _, dir := range dirs {
			if p := firstExecutable(dir, names); p != "" {
				return Choice{Argv: []string{p, "--wait"}, Source: "remote-cli", Announce: true}, true
			}
		}
	}
	for _, name := range names {
		p, err := exec.LookPath(name)
		if err == nil && executableFile(p) {
			return Choice{Argv: []string{p, "--wait"}, Source: "remote-cli", Announce: true}, true
		}
	}
	return Choice{}, false
}

func preferredCLINames() []string {
	// TERM_PROGRAM names the product. The IPC hook path is not a preference:
	// a Cursor hook with no matching TERM_PROGRAM still searches
	// code-server, code, cursor, codium.
	prog := strings.ToLower(strings.TrimSpace(os.Getenv("TERM_PROGRAM")))
	var first []string
	switch prog {
	case "cursor":
		first = []string{"cursor"}
	case "codium", "vscodium":
		first = []string{"codium"}
	case "code-server":
		first = []string{"code-server", "code"}
	case "vscode", "code":
		first = []string{"code", "code-server"}
	}
	rest := []string{"code-server", "code", "cursor", "codium"}
	seen := map[string]bool{}
	var out []string
	for _, n := range append(first, rest...) {
		if seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

func desktopGUI() (Choice, bool) {
	candidates := []struct {
		name string
		args []string
	}{
		{"cursor", []string{"--wait"}},
		{"code", []string{"--wait"}},
		{"code-server", []string{"--wait"}},
		{"codium", []string{"--wait"}},
		{"subl", []string{"-w"}},
		{"gedit", []string{"-s"}},
		{"kate", []string{"--block"}},
	}
	// Prefer the CLI that matches the desktop session's editor, then the rest.
	names := preferredCLINames()
	var ordered []struct {
		name string
		args []string
	}
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	for _, n := range names {
		for _, c := range candidates {
			if c.name == n {
				ordered = append(ordered, c)
			}
		}
	}
	for _, c := range candidates {
		if !want[c.name] {
			ordered = append(ordered, c)
		}
	}
	for _, c := range ordered {
		p, err := exec.LookPath(c.name)
		if err != nil || !executableFile(p) {
			continue
		}
		argv := append([]string{p}, c.args...)
		return Choice{Argv: argv, Source: "desktop", Announce: true}, true
	}
	return Choice{}, false
}

func firstExecutable(dir string, names []string) string {
	for _, n := range names {
		p := filepath.Join(dir, n)
		if executableFile(p) {
			return p
		}
	}
	return ""
}

func executableFile(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return false
	}
	return fi.Mode()&0o111 != 0
}

func announces(fields []string) bool {
	return len(fields) > 0 && isGUIFamily(filepath.Base(fields[0])) && hasWaitFlag(filepath.Base(fields[0]), fields[1:])
}

func skipWithoutTTY(fields []string) string {
	base := filepath.Base(fields[0])
	if isTerminalEditor(base, fields[1:]) {
		return base + ", which needs a terminal"
	}
	if isNonWaitingOpener(base, fields[1:]) {
		return base + ", which returns before the file is closed"
	}
	if isGUIFamily(base) && !hasWaitFlag(base, fields[1:]) {
		return base + " without a wait flag"
	}
	return ""
}

func isTerminalEditor(base string, args []string) bool {
	switch base {
	case "vim", "nvim", "vi", "nano", "pico", "joe", "mg", "hx", "helix", "micro":
		return true
	case "emacs":
		for _, a := range args {
			if a == "-nw" || a == "--no-window" || a == "--tty" || strings.HasPrefix(a, "-nw") {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func isNonWaitingOpener(base string, args []string) bool {
	switch base {
	case "xdg-open":
		return true
	case "open":
		return !hasAny(args, "-W", "--wait", "--wait-apps")
	default:
		return false
	}
}

func isGUIFamily(base string) bool {
	switch base {
	case "code", "code-insiders", "code-oss", "code-server", "cursor", "codium", "subl", "sublime_text", "gedit", "kate":
		return true
	default:
		return false
	}
}

func hasWaitFlag(base string, args []string) bool {
	switch base {
	case "gedit":
		return hasAny(args, "-s", "--standalone", "--wait")
	case "kate":
		return hasAny(args, "-b", "--block")
	default:
		return hasAny(args, "--wait", "-w", "-W")
	}
}

func hasAny(args []string, flags ...string) bool {
	for _, a := range args {
		for _, f := range flags {
			if a == f {
				return true
			}
		}
	}
	return false
}

// CommandLine joins a choice for Edit.
func (c Choice) CommandLine() string {
	return strings.Join(c.Argv, " ")
}
