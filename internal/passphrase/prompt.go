package passphrase

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	writeWaysForward(&b)
	return b.String()
}

// EditorTimeoutSummary is the first line when the editor accepted the request
// and did not answer. It stays stable so a person or an agent can grep for it.
const EditorTimeoutSummary = "pogo: the editor prompt got no answer within the timeout"

// PromptTimeoutError means a prompt was shown and nobody answered before the
// prompt timeout. Callers stop. They do not open another dialog.
// What is "editor", "POGO_ASKPASS", or a desktop helper label.
type PromptTimeoutError struct {
	What string
	Wait time.Duration
}

// EditorTimeoutError is the editor case of PromptTimeoutError.
type EditorTimeoutError = PromptTimeoutError

func (e *PromptTimeoutError) Error() string {
	wait := promptTimeout
	what := "editor"
	if e != nil {
		if e.Wait > 0 {
			wait = e.Wait
		}
		if e.What != "" {
			what = e.What
		}
	}
	kind := "editor prompt"
	subject := "The editor"
	switch what {
	case "editor":
	case "POGO_ASKPASS":
		kind = "POGO_ASKPASS prompt"
		subject = "POGO_ASKPASS"
	default:
		kind = what + " prompt"
		subject = what
	}
	var b strings.Builder
	fmt.Fprintf(&b, "pogo: the %s got no answer within the timeout\n", kind)
	fmt.Fprintf(&b, "%s was asked and did not answer within %s. Pogo did not open another dialog.\n", subject, wait)
	writeWaysForward(&b)
	return b.String()
}

func finishedPrompt(err error) bool {
	if err == nil || errors.Is(err, ErrCancelled) || errors.Is(err, ErrTooLong) {
		return true
	}
	var timed *PromptTimeoutError
	return errors.As(err, &timed)
}

func writeWaysForward(b *strings.Builder) {
	b.WriteString("Ways forward, in order:\n")
	b.WriteString("  1. Run this command yourself in a terminal.\n")
	b.WriteString("  2. In Docker, use docker run -it, or docker compose run, or docker compose up with stdin_open: true and tty: true and then docker attach.\n")
	b.WriteString("  3. Pipe the passphrase with --key-stdin from a secret manager or an OS keychain CLI. The program cannot also read stdin.\n")
	b.WriteString("  4. Set POGO_ASKPASS to a program that asks and prints one line on stdout.\n")
	b.WriteString("  5. Last resort: --key-env NAME. The value stays visible in /proc and docker inspect.\n")
	b.WriteString(AgentLine)
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
//   - The dialog timeout (the process was started and did not exit): the
//     prompt got no answer. Pogo stops with PromptTimeoutError. It does not
//     open another dialog. A non-zero exit, including Escape, stays a cancel.
//
// POGO_ASKPASS follows the SSH_ASKPASS contract: exit 0 submits, any other
// exit cancels and stops. A timeout is "no answer", not a cancel. If that
// program cannot be started, it cannot show a prompt, so Pogo falls through.
//
// The editor channel falls through on a missing socket, a refused connection,
// a dial timeout, or a non-200. Those never showed a box. Cancel is only
// HTTP 200 with an empty body or a JSON "". An empty 200 does not fall
// through to a desktop dialog.
//
// A response timeout is different. The request was sent and the editor may
// still be showing the box, so Pogo stops. It does not open a desktop dialog
// two minutes later. The error is PromptTimeoutError, the same error a
// POGO_ASKPASS or desktop helper timeout returns.
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

func promptOutOfBand(prompt, file string, kind PromptKind) ([]byte, error) {
	debug("stdin is not a terminal")
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
		if finishedPrompt(err) {
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
		pass, err := askEditor(sock, text, file, kind)
		if finishedPrompt(err) {
			return pass, err
		}
		var skipped *editorSkip
		if errors.As(err, &skipped) {
			note(skipped.reason)
		} else {
			note("editor prompt failed")
		}
	}

	if ok, skip := displayDecision(); !ok {
		note(skip)
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
		if finishedPrompt(err) {
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

// NoXSocketCheck is the Checked: line when DISPLAY or WAYLAND_DISPLAY names
// a local server whose socket is not there.
const NoXSocketCheck = "display set but no X server socket"

// x11SocketDir is where local displays :N keep their sockets (XN).
// Tests point it at a fake directory.
var x11SocketDir = "/tmp/.X11-unix"

// SetX11DirForTest points local DISPLAY socket checks at dir.
func SetX11DirForTest(dir string) func() {
	old := x11SocketDir
	x11SocketDir = dir
	return func() { x11SocketDir = old }
}

// DisplayReady reports whether a desktop window can be opened.
// The string is the reason when it cannot, the same text doctor prints.
func DisplayReady() (bool, string) {
	return displayDecision()
}

// displayDecision reports whether a desktop askpass should be tried.
// A local DISPLAY (:N or unix:N) needs /tmp/.X11-unix/XN. A relative
// WAYLAND_DISPLAY needs that socket under XDG_RUNTIME_DIR. An absolute
// Wayland path must exist. TCP displays such as localhost:10.0 are left
// alone, because SSH forwarding does not put a socket in /tmp/.X11-unix.
func displayDecision() (bool, string) {
	wayland := os.Getenv("WAYLAND_DISPLAY")
	display := os.Getenv("DISPLAY")
	mir := os.Getenv("MIR_SOCKET")
	if wayland == "" && display == "" && mir == "" {
		return false, noDisplayReason()
	}
	if waylandReady(wayland) || x11Ready(display) || mirReady(mir) {
		return true, ""
	}
	return false, NoXSocketCheck
}

func waylandReady(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	if filepath.IsAbs(name) {
		return pathExists(name)
	}
	runtime := os.Getenv("XDG_RUNTIME_DIR")
	if runtime == "" {
		return false
	}
	return pathExists(filepath.Join(runtime, name))
}

func x11Ready(display string) bool {
	display = strings.TrimSpace(display)
	if display == "" {
		return false
	}
	num, local := localXNumber(display)
	if !local {
		// TCP or a forwarded display. Do not require a local socket.
		return true
	}
	return pathExists(filepath.Join(x11SocketDir, "X"+num))
}

func mirReady(path string) bool {
	path = strings.TrimSpace(path)
	return path != "" && pathExists(path)
}

// localXNumber reports the display number for :N, :N.S, unix:N, or unix/:N.
// Anything else, including localhost:10.0, is not a local socket display.
func localXNumber(display string) (string, bool) {
	s := strings.TrimSpace(display)
	switch {
	case strings.HasPrefix(s, "unix/:"):
		s = strings.TrimPrefix(s, "unix/")
	case strings.HasPrefix(s, "unix:"):
		s = strings.TrimPrefix(s, "unix:")
		if !strings.HasPrefix(s, ":") {
			s = ":" + s
		}
	}
	if !strings.HasPrefix(s, ":") {
		return "", false
	}
	s = s[1:]
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return "", false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	return s, true
}

func pathExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

// dialogTimeout is how long an out-of-band prompt may stay unanswered.
// POGO_PROMPT_TIMEOUT is a positive number of seconds. Unset or invalid
// keeps the default (two minutes, or the test override).
func dialogTimeout() time.Duration {
	v := os.Getenv("POGO_PROMPT_TIMEOUT")
	if v == "" {
		return promptTimeout
	}
	sec, err := strconv.Atoi(v)
	if err != nil || sec <= 0 {
		debug("POGO_PROMPT_TIMEOUT is not a positive number of seconds; using the default")
		return promptTimeout
	}
	return time.Duration(sec) * time.Second
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

	ctx, cancel := contextWithTimeout(dialogTimeout())
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
		// The helper was started and may still be on screen. This is not a
		// cancel: nobody answered. Stop, and do not open another dialog.
		what := "POGO_ASKPASS"
		if mode == modeDesktop {
			what = label
		}
		return nil, &PromptTimeoutError{What: what, Wait: dialogTimeout()}
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
