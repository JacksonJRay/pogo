package passphrase

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"
	"pogo/internal/securemem"
)

// HerdrLabel is the doctor backend name when Pogo asks in a Herdr pane.
// Ovis, the Herdr UI, shows its masked Password or PIN field for that pane.
const HerdrLabel = "Herdr"

// herdrAvailable reports whether this process is inside a Herdr pane and the
// herdr CLI can open another pane. An empty reason means Herdr is not in
// play, so callers leave it off the checked list.
func herdrAvailable() (bool, string) {
	if os.Getenv("HERDR_ENV") != "1" {
		return false, ""
	}
	if strings.TrimSpace(os.Getenv("HERDR_PANE_ID")) == "" {
		return false, "Herdr is set but HERDR_PANE_ID is empty"
	}
	if _, err := exec.LookPath("herdr"); err != nil {
		return false, "herdr program not found"
	}
	return true, ""
}

// askHerdr opens a temporary Herdr pane and reads one passphrase from it.
// The pane's last line is an Ovis secret prompt. The passphrase comes back
// on a private socket. It is not typed into the caller's pane.
func askHerdr(hint, line string) ([]byte, error) {
	if strings.ContainsAny(hint, "\r\n") || strings.ContainsAny(line, "\r\n") || strings.TrimSpace(line) == "" {
		return nil, errNotStarted
	}
	bin, err := os.Executable()
	if err != nil || bin == "" {
		return nil, errNotStarted
	}
	dir, err := os.MkdirTemp("", "pogo-herdr-")
	if err != nil {
		return nil, errNotStarted
	}
	defer os.RemoveAll(dir)
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, errNotStarted
	}
	sockPath := filepath.Join(dir, "sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return nil, errNotStarted
	}
	defer ln.Close()
	if err := os.Chmod(sockPath, 0o600); err != nil {
		return nil, errNotStarted
	}

	script := filepath.Join(dir, "ask.sh")
	body := "#!/bin/sh\n" +
		"export POGO_HERDR_RESPONDER=1\n" +
		"export POGO_HERDR_SOCKET=" + shSingle(sockPath) + "\n" +
		"export POGO_HERDR_PROMPT=" + shSingle(line) + "\n" +
		"export POGO_HERDR_HINT=" + shSingle(hint) + "\n" +
		"exec " + shSingle(bin) + "\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		return nil, errNotStarted
	}

	fmt.Fprintf(os.Stderr, "Asking for the passphrase with %s\n", HerdrLabel)
	fmt.Fprintf(os.Stderr, "Ovis shows a masked Password or PIN field in the Herdr pane.\n")
	debug("using Herdr pane")

	wait := dialogTimeout()
	ctx, cancel := contextWithTimeout(wait)
	defer cancel()

	pane, err := herdrSplit(ctx)
	if err != nil {
		debug("Herdr pane could not be opened")
		return nil, errNotStarted
	}
	defer herdrClose(pane)

	type accepted struct {
		conn net.Conn
		err  error
	}
	acc := make(chan accepted, 1)
	go func() {
		if ul, ok := ln.(*net.UnixListener); ok {
			_ = ul.SetDeadline(time.Now().Add(wait))
		}
		conn, err := ln.Accept()
		acc <- accepted{conn, err}
	}()

	runDone := make(chan error, 1)
	go func() {
		runDone <- herdrRun(ctx, pane, script)
	}()

	take := func(got accepted) ([]byte, error) {
		if got.err != nil || got.conn == nil {
			if ctx.Err() != nil {
				return nil, &PromptTimeoutError{What: HerdrLabel, Wait: wait}
			}
			return nil, errNotStarted
		}
		defer got.conn.Close()
		_ = got.conn.SetDeadline(time.Now().Add(5 * time.Second))
		return readHerdrSocket(got.conn)
	}
	select {
	case got := <-acc:
		return take(got)
	case <-runDone:
		timer := time.NewTimer(100 * time.Millisecond)
		defer timer.Stop()
		select {
		case got := <-acc:
			return take(got)
		case <-ctx.Done():
			return nil, &PromptTimeoutError{What: HerdrLabel, Wait: wait}
		case <-timer.C:
			debug("Herdr pane command failed")
			return nil, errNotStarted
		}
	case <-ctx.Done():
		return nil, &PromptTimeoutError{What: HerdrLabel, Wait: wait}
	}
}

func herdrSplit(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "herdr", "pane", "split", "--current", "--direction", "down", "--focus")
	prepareCommand(cmd)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	raw := out
	if i := strings.IndexByte(string(out), '{'); i >= 0 {
		raw = out[i:]
	}
	var resp struct {
		Result struct {
			Pane struct {
				PaneID string `json:"pane_id"`
			} `json:"pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	id := strings.TrimSpace(resp.Result.Pane.PaneID)
	if id == "" {
		return "", errors.New("herdr split did not return a pane id")
	}
	return id, nil
}

func herdrRun(ctx context.Context, pane, script string) error {
	var last error
	for try := 0; try < 15; try++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		cmd := exec.CommandContext(ctx, "herdr", "pane", "run", pane, script)
		prepareCommand(cmd)
		_, err := cmd.Output()
		if err == nil {
			return nil
		}
		last = err
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	if last == nil {
		last = errors.New("herdr pane run failed")
	}
	return last
}

func herdrClose(pane string) {
	if pane == "" {
		return
	}
	ctx, cancel := contextWithTimeout(5 * time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "herdr", "pane", "close", pane)
	prepareCommand(cmd)
	_ = cmd.Run()
}

func readHerdrSocket(conn net.Conn) ([]byte, error) {
	r := bufio.NewReaderSize(conn, MaxBytes+8)
	status, err := readLimitedLine(r)
	if err != nil {
		return nil, err
	}
	if string(status) == "cancel" {
		return nil, ErrCancelled
	}
	if string(status) != "ok" {
		return nil, errNotStarted
	}
	pass, err := readLimitedLine(r)
	if err != nil {
		securemem.Wipe(pass)
		return nil, err
	}
	if len(pass) > MaxBytes {
		securemem.Wipe(pass)
		return nil, ErrTooLong
	}
	return pass, nil
}

func readLimitedLine(r *bufio.Reader) ([]byte, error) {
	var buf []byte
	for {
		if len(buf) > MaxBytes {
			securemem.Wipe(buf)
			return nil, ErrTooLong
		}
		b, err := r.ReadByte()
		if err != nil {
			securemem.Wipe(buf)
			return nil, err
		}
		if b == '\n' {
			return buf, nil
		}
		if b == '\r' {
			continue
		}
		buf = append(buf, b)
	}
}

func shSingle(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// HerdrResponder is the process Herdr runs inside the temporary pane.
// It prints the Ovis prompt on that pane's terminal and sends one answer
// to the private socket. A pipe is refused so an agent cannot supply the
// passphrase on stdin.
func HerdrResponder() int {
	sock := os.Getenv("POGO_HERDR_SOCKET")
	line := os.Getenv("POGO_HERDR_PROMPT")
	hint := os.Getenv("POGO_HERDR_HINT")
	if sock == "" || strings.TrimSpace(line) == "" || strings.ContainsAny(line, "\r\n") || strings.ContainsAny(hint, "\r\n") {
		return 1
	}
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		fmt.Fprintln(os.Stderr, "pogo: the Herdr pane has no terminal to ask on")
		return 1
	}
	if hint != "" {
		fmt.Fprintln(os.Stderr, hint)
	}
	fmt.Fprint(os.Stderr, line)
	pw, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	sendHerdrAnswer(sock, pw, err)
	securemem.Wipe(pw)
	if err != nil {
		return 1
	}
	return 0
}

func sendHerdrAnswer(sock string, pw []byte, readErr error) {
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if readErr != nil {
		_, _ = conn.Write([]byte("cancel\n"))
		return
	}
	if len(pw) > MaxBytes {
		_, _ = conn.Write([]byte("cancel\n"))
		return
	}
	_, _ = conn.Write([]byte("ok\n"))
	_, _ = conn.Write(pw)
	_, _ = conn.Write([]byte("\n"))
}
