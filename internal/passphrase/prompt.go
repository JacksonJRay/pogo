package passphrase

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"pogo/internal/securemem"
)

// Timeouts for out-of-band prompts. Dial is short so a dead editor socket
// cannot stall detection. The dialog timeout bounds a prompt the person
// never answers. Tests replace these through SetPromptTimeoutsForTest.
var (
	dialTimeout   = 2 * time.Second
	promptTimeout = 2 * time.Minute
)

// ErrCancelled means a prompt was shown (or a chosen helper finished) and
// the person dismissed it, submitted nothing, or the helper exited without
// a display failure. Callers stop. They do not open another dialog.
var ErrCancelled = errors.New("passphrase prompt cancelled")

// NoPromptError means stdin is not a terminal and no backend could show a prompt.
// Checks lists what was considered, in detection order, without secret values.
type NoPromptError struct {
	Checks []string
}

// NoPromptSummary is the first line of the exit-1 message. It stays stable
// so a person or an agent can grep for it.
const NoPromptSummary = "pogo: no way to ask for the passphrase here"

// AgentLine tells an agent not to collect the passphrase in chat.
const AgentLine = "If you are an AI agent: do not ask the person to paste the passphrase into chat; ask them to run this command in their own terminal."

func (e *NoPromptError) Error() string {
	if e == nil {
		return FormatNoPrompt(nil)
	}
	return FormatNoPrompt(e.Checks)
}

// FormatNoPrompt is the agent-readable and human-readable exit-1 text.
// Ways forward are in priority order. --key-env is labeled last resort.
func FormatNoPrompt(checks []string) string {
	var b strings.Builder
	b.WriteString(NoPromptSummary)
	b.WriteByte('\n')
	if len(checks) == 0 {
		b.WriteString("Nothing could show a prompt.\n")
	} else {
		b.WriteString("Checked:\n")
		for _, c := range checks {
			b.WriteString("  ")
			b.WriteString(c)
			b.WriteByte('\n')
		}
	}
	b.WriteString("Ways forward, in order:\n")
	b.WriteString("  1. Run this command yourself in a terminal.\n")
	b.WriteString("  2. In Docker, use docker run -it, or docker compose run, or docker compose up with stdin_open: true and tty: true and then docker attach.\n")
	b.WriteString("  3. Pipe the passphrase with --key-stdin from a secret manager or an OS keychain CLI. The program cannot also read stdin.\n")
	b.WriteString("  4. Set POGO_ASKPASS to a program that asks and prints one line on stdout.\n")
	b.WriteString("  5. Last resort: --key-env NAME. The value stays visible in /proc and docker inspect.\n")
	b.WriteString(AgentLine)
	return b.String()
}

// How a desktop helper's non-zero exit is classified:
//
//   - Stderr or stdout contains a known display-connection phrase
//     ("cannot open display", a Wayland connect failure, X11 "no protocol
//     specified", and the phrases in displayFailurePhrases): the helper
//     never showed a dialog. The same list includes the gcr relay failures
//     ("not meant to be run directly", "couldn't connect to askpass socket").
//     The helper is unavailable. Pogo tries the next helper.
//   - Any other non-zero exit, including a typical cancel button: the person
//     cancelled. Pogo stops. A cancel dialog and a broken helper look the
//     same unless the helper names a display error, so this is the rule.
//   - The dialog timeout (the process was started and did not exit): cancel
//     and stop. A second dialog after a missed one is worse than stopping.
//
// POGO_ASKPASS follows the SSH_ASKPASS contract instead: exit 0 submits,
// any other exit or a timeout cancels and stops. If that program cannot be
// started, it cannot show a prompt, so Pogo falls through.
//
// The editor channel falls through on a missing socket, a refused connection,
// a dial timeout, a response timeout, or a non-200. Those are a dead editor,
// not a cancel. Cancel is only HTTP 200 with an empty body or a JSON "".
// An empty 200 does not fall through to a desktop dialog.
var errDisplay = errors.New("askpass could not open a display")

type editorSkip struct{ reason string }

func (e *editorSkip) Error() string { return e.reason }

var errNotStarted = errors.New("askpass program could not be started")

// SetPromptTimeoutsForTest overrides the dial and dialog timeouts.
// The returned function restores the previous values.
func SetPromptTimeoutsForTest(dial, overall time.Duration) func() {
	oldDial, oldOverall := dialTimeout, promptTimeout
	dialTimeout, promptTimeout = dial, overall
	return func() {
		dialTimeout, promptTimeout = oldDial, oldOverall
	}
}

// dirOverrideOn, when set, limits desktop helper lookups to dirOverride.
// Production leaves it false and uses the distro libexec directories.
var (
	dirOverrideOn bool
	dirOverride   []string
)

// SetAskpassDirsForTest limits desktop helper search to dirs.
// An empty slice searches no extra directories (PATH is still consulted).
func SetAskpassDirsForTest(dirs []string) func() {
	oldOn, old := dirOverrideOn, dirOverride
	dirOverrideOn = true
	dirOverride = append([]string(nil), dirs...)
	return func() {
		dirOverrideOn, dirOverride = oldOn, old
	}
}

func promptOutOfBand(prompt, file string) ([]byte, error) {
	text := withFile(prompt, file)
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
		debug("using POGO_ASKPASS")
		pass, err := runProgram(filepath.Base(path), []string{path, text}, modeAskpass)
		if err == nil || errors.Is(err, ErrCancelled) || errors.Is(err, ErrTooLong) {
			return pass, err
		}
		if errors.Is(err, errNotStarted) {
			note("POGO_ASKPASS could not be started")
		} else {
			note("POGO_ASKPASS failed")
		}
	}

	if sock, skip := resolveEditorSocket(); sock == "" {
		note(skip)
	} else {
		pass, err := askEditor(sock, text)
		if err == nil || errors.Is(err, ErrCancelled) || errors.Is(err, ErrTooLong) {
			return pass, err
		}
		var skipped *editorSkip
		if errors.As(err, &skipped) {
			note(skipped.reason)
		} else {
			note("editor prompt failed")
		}
	}

	if !displayPresent() {
		note(noDisplayReason())
		return nil, &NoPromptError{Checks: checks}
	}

	helpers := findHelpers()
	if len(helpers) == 0 {
		note("no desktop askpass found")
		return nil, &NoPromptError{Checks: checks}
	}
	for _, h := range helpers {
		if reason := helperBlock(h); reason != "" {
			note(reason)
			continue
		}
		pass, err := runProgram(h.label, h.argv(text), modeDesktop)
		if err == nil || errors.Is(err, ErrCancelled) || errors.Is(err, ErrTooLong) {
			return pass, err
		}
		if errors.Is(err, errDisplay) {
			note(h.label + " could not open a display")
			continue
		}
		if errors.Is(err, errNotStarted) {
			note(h.label + " could not be started")
			continue
		}
		note(h.label + " failed")
	}
	return nil, &NoPromptError{Checks: checks}
}

func displayPresent() bool {
	for _, name := range []string{"WAYLAND_DISPLAY", "DISPLAY", "MIR_SOCKET"} {
		if os.Getenv(name) != "" {
			return true
		}
	}
	return false
}

func noDisplayReason() string {
	switch {
	case os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_CLIENT") != "":
		return "no display (SSH session)"
	case os.Getenv("WSL_DISTRO_NAME") != "" || os.Getenv("WSL_INTEROP") != "":
		return "no display (WSL without WSLg)"
	default:
		return "no display"
	}
}

// askpassProgram returns the POGO_ASKPASS executable. An empty value is not set.
// A name that is not executable cannot show a prompt, so the caller falls through.
func askpassProgram() (string, string) {
	path := os.Getenv("POGO_ASKPASS")
	if path == "" {
		return "", "POGO_ASKPASS not set"
	}
	if strings.Contains(path, "/") {
		if executable(path) {
			return path, ""
		}
		return "", "POGO_ASKPASS could not be started"
	}
	if os.Getenv("PATH") == "" {
		return "", "POGO_ASKPASS could not be started"
	}
	found, err := exec.LookPath(path)
	if err != nil || !executable(found) {
		return "", "POGO_ASKPASS could not be started"
	}
	return found, ""
}

func withFile(prompt, file string) string {
	if prompt == "" {
		prompt = "Enter passphrase: "
	}
	file = strings.TrimSpace(file)
	if file == "" || strings.Contains(prompt, file) {
		return prompt
	}
	trimmed := strings.TrimRight(prompt, " \t")
	if strings.HasSuffix(trimmed, ":") {
		trimmed = strings.TrimRight(strings.TrimSuffix(trimmed, ":"), " \t")
		return trimmed + " for " + file + ": "
	}
	return trimmed + " for " + file
}

// editorPlaceholder is the Git input-box placeholder. The workbench masks
// the box only when this text matches /password/i. "passphrase" does not.
func editorPlaceholder(prompt string) string {
	if strings.Contains(strings.ToLower(prompt), "password") {
		return prompt
	}
	return strings.TrimRight(prompt, " \t") + " (password)"
}

const editorHost = "https://pogo.invalid"

type runMode int

const (
	modeAskpass runMode = iota
	modeDesktop
)

func runProgram(label string, argv []string, mode runMode) ([]byte, error) {
	if len(argv) == 0 || argv[0] == "" {
		return nil, errNotStarted
	}
	fmt.Fprintf(os.Stderr, "Asking for the passphrase with %s\n", label)
	debug("running " + label)

	ctx, cancel := contextWithTimeout(promptTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	prepareCommand(cmd)
	cmd.Stdin = nil
	var stdout, stderr capBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	defer stdout.wipe()
	defer stderr.wipe()

	if timedOut(ctx) {
		// A helper we started may still be on screen. Stop, do not open another.
		return nil, ErrCancelled
	}
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || isExecStartFailure(err) {
			return nil, errNotStarted
		}
		if mode == modeDesktop && isDisplayFailure(stderr.bytes(), stdout.bytes()) {
			return nil, errDisplay
		}
		return nil, ErrCancelled
	}
	if stdout.tooLong {
		return nil, ErrTooLong
	}
	return decodePassphrase(stdout.bytes())
}

func isExecStartFailure(err error) bool {
	if err == nil {
		return false
	}
	// A process that started and then exited, including exit 126 or 127 from
	// the helper itself, is a finished prompt. Only a failure to exec falls
	// through. The SSH_ASKPASS contract treats any other non-zero exit as cancel.
	var exited *exec.ExitError
	if errors.As(err, &exited) {
		return false
	}
	if errors.Is(err, exec.ErrNotFound) {
		return true
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return true
	}
	var execErr *exec.Error
	if errors.As(err, &execErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "executable file not found") || strings.Contains(msg, "permission denied") || strings.Contains(msg, "not a directory") || strings.Contains(msg, "exec format error")
}

func decodePassphrase(buf []byte) ([]byte, error) {
	pass := stripOneNewline(buf)
	if len(pass) > MaxBytes {
		return nil, ErrTooLong
	}
	if len(pass) == 0 {
		return nil, ErrCancelled
	}
	out := make([]byte, len(pass))
	copy(out, pass)
	return out, nil
}

func stripOneNewline(b []byte) []byte {
	if len(b) > 0 && b[len(b)-1] == '\n' {
		return b[:len(b)-1]
	}
	return b
}

var displayFailurePhrases = []string{
	"cannot open display",
	"can't open display",
	"could not open display",
	"could not connect to display",
	"cannot connect to display",
	"unable to open display",
	"error opening display",
	"no protocol specified",
	"authorization required, but no authorization protocol",
	"could not connect to wayland",
	"cannot connect to wayland",
	"failed to connect to wayland",
	"failed to connect to the wayland",
	"unable to init server",
	"qt.qpa.xcb: could not connect to display",
	// gcr4-ssh-askpass relays to GCR_SSH_ASKPASS_SOCKET. These mean it
	// never showed a dialog, so the next helper can be tried.
	"not meant to be run directly",
	"couldn't connect to askpass socket",
	"could not connect to askpass socket",
	"couldn't open socket",
}

func isDisplayFailure(chunks ...[]byte) bool {
	var b strings.Builder
	for _, c := range chunks {
		b.Write(c)
		b.WriteByte('\n')
	}
	text := strings.ToLower(b.String())
	for _, phrase := range displayFailurePhrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

type capBuffer struct {
	b       []byte
	tooLong bool
}

func (c *capBuffer) Write(p []byte) (int, error) {
	if c.tooLong {
		return len(p), nil
	}
	c.b = append(c.b, p...)
	// One extra byte so a single trailing newline past MaxBytes is still
	// visible to stripOneNewline before the length check.
	if len(c.b) > MaxBytes+1 {
		c.tooLong = true
		securemem.Wipe(c.b)
		c.b = nil
	}
	return len(p), nil
}

func (c *capBuffer) bytes() []byte { return c.b }

func (c *capBuffer) wipe() {
	securemem.Wipe(c.b)
	c.b = nil
}

func debug(msg string) {
	if os.Getenv("POGO_DEBUG") != "1" {
		return
	}
	fmt.Fprintf(os.Stderr, "pogo: %s\n", msg)
}
