package passphrase

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// MacDialogLabel is the doctor backend and the stderr helper name.
const MacDialogLabel = "macOS dialog"

// MacDialogTitle is the title of the macOS password window.
const MacDialogTitle = "Pogo Password Request"

// goos is the OS used for prompt selection. Tests set it.
var goos = runtime.GOOS

// osascriptPath is the macOS dialog helper. Tests point it at a fake.
var osascriptPath = "/usr/bin/osascript"

// consoleDevice is the GUI login device. Its owner is the person at the screen.
var consoleDevice = "/dev/console"

// macDialogOff skips the macOS dialog. Tests set it so a developer machine
// does not open a real window.
var macDialogOff bool

// SetMacDialogForTest turns the macOS dialog off or back on.
func SetMacDialogForTest(off bool) func() {
	old := macDialogOff
	macDialogOff = off
	return func() { macDialogOff = old }
}

// agentSession reports whether this process was started by an agent that
// cannot type a passphrase. A person at a real terminal is not one of these.
func agentSession() bool {
	if os.Getenv("POGO_FORCE_TERMINAL") == "1" {
		return false
	}
	if os.Getenv("AGENT") == "1" {
		return true
	}
	for _, key := range []string{
		"OPENCODE",
		"CLAUDECODE",
		"CLAUDE_CODE",
		"CURSOR_AGENT",
		"CODEX_SANDBOX",
		"CODEX_THREAD_ID",
		"GEMINI_CLI",
		"AIDER",
	} {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return true
		}
	}
	return false
}

func useMacDialogInsteadOfTerminal() bool {
	if !agentSession() {
		return false
	}
	ok, _ := macDialogAvailable()
	return ok
}

// macDialogAvailable reports whether a macOS password window can be shown.
// An empty reason means this step does not apply and should not be listed.
func macDialogAvailable() (bool, string) {
	if macDialogOff || goos != "darwin" {
		return false, ""
	}
	if os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_CLIENT") != "" {
		return false, ""
	}
	if !executable(osascriptPath) {
		return false, "macOS dialog not found"
	}
	uid, ok := consoleOwnerFn()
	if !ok || uid == 0 || int(uid) != os.Getuid() {
		return false, "no macOS GUI session"
	}
	return true, ""
}

var consoleOwnerFn = consoleOwner

func consoleOwner() (uint32, bool) {
	fi, err := os.Stat(consoleDevice)
	if err != nil {
		return 0, false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return st.Uid, true
}

func macDialogBody(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		prompt = "Enter the passphrase."
	}
	return MacDialogTitle + "\n\n" + prompt
}

func macDialogScript(prompt string, seconds int) string {
	if seconds < 1 {
		seconds = 1
	}
	return "set d to display dialog (" + appleString(macDialogBody(prompt)) + `) default answer "" with hidden answer with title ` + appleString(MacDialogTitle) + ` buttons {"Cancel", "OK"} default button "OK" cancel button "Cancel" giving up after ` + strconv.Itoa(seconds) + `
if gave up of d is true then
error "pogo timeout" number 2
end if
return text returned of d
`
}

func appleString(s string) string {
	parts := strings.Split(s, "\n")
	var b strings.Builder
	for i, part := range parts {
		if i > 0 {
			b.WriteString(" & return & ")
		}
		b.WriteByte('"')
		b.WriteString(strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(part))
		b.WriteByte('"')
	}
	if b.Len() == 0 {
		return `""`
	}
	return b.String()
}

func askMacDialog(prompt string) ([]byte, error) {
	if !executable(osascriptPath) {
		return nil, errNotStarted
	}
	secs := int(dialogTimeout() / time.Second)
	script := macDialogScript(prompt, secs)
	fmt.Fprintf(os.Stderr, "Asking for the passphrase with %s\n", MacDialogLabel)
	fmt.Fprintf(os.Stderr, "The window is titled %q.\n", MacDialogTitle)

	ctx, cancel := contextWithTimeout(dialogTimeout() + 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, osascriptPath, "-e", script)
	prepareCommand(cmd)
	cmd.Stdin = nil
	var stdout, stderr capBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	defer stdout.wipe()
	defer stderr.wipe()

	if macTimedOut(stderr.bytes(), stdout.bytes()) || timedOut(ctx) {
		return nil, &PromptTimeoutError{What: MacDialogLabel, Wait: dialogTimeout()}
	}
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || isExecStartFailure(err) {
			return nil, errNotStarted
		}
		if macNotShown(stderr.bytes(), stdout.bytes()) {
			return nil, errDisplay
		}
		return nil, ErrCancelled
	}
	if stdout.tooLong {
		return nil, ErrTooLong
	}
	return decodePassphrase(stdout.bytes())
}

func macTimedOut(chunks ...[]byte) bool {
	text := strings.ToLower(string(bytesJoin(chunks)))
	return strings.Contains(text, "pogo timeout") || strings.Contains(text, "gave up:true")
}

func macNotShown(chunks ...[]byte) bool {
	text := strings.ToLower(string(bytesJoin(chunks)))
	for _, phrase := range []string{
		"no user interaction allowed",
		"(-1713)",
		"(-610)",
		"not authorized to send apple events",
		"(-1743)",
		"not allowed assistive access",
	} {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

func bytesJoin(chunks [][]byte) []byte {
	n := 0
	for _, c := range chunks {
		n += len(c) + 1
	}
	out := make([]byte, 0, n)
	for _, c := range chunks {
		out = append(out, c...)
		out = append(out, '\n')
	}
	return out
}
