package passphrase

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pogo/internal/securemem"
)

// askEditor POSTs to the Git extension IPC socket (VSCODE_GIT_IPC_HANDLE).
// code-server and VS Code want {askpassType, argv}. Cursor's helper sends
// {askpassType, request, host} and the header x-cursor-git-auth-token when
// VSCODE_GIT_IPC_AUTH_TOKEN is set.
//
// argv[2] is the placeholder and contains "password" so the box is masked.
// argv[4] is a URL whose host is pogo.invalid, so a cached Git password for
// some other host is not returned. The workbench titles the box from that
// URL; there is no generic prompt API.
// resolveEditorSocket finds the Git IPC socket. VSCODE_GIT_IPC_HANDLE is the
// normal path, including Remote SSH and code-server. A Flatpak or Snap build
// sometimes injects a host path that is not visible inside the sandbox; the
// socket is then under XDG_RUNTIME_DIR, either with the same basename or as
// the only vscode-git-*.sock. Several matches are ambiguous and are skipped.
func resolveEditorSocket() (string, string) {
	raw := os.Getenv("VSCODE_GIT_IPC_HANDLE")
	if raw == "" {
		return "", "editor socket not set"
	}
	if usableSocket(raw) {
		return raw, ""
	}
	runtime := os.Getenv("XDG_RUNTIME_DIR")
	if runtime != "" {
		base := filepath.Base(raw)
		if base != "." && base != string(filepath.Separator) && base != "" {
			cand := filepath.Join(runtime, base)
			if usableSocket(cand) {
				debug("editor socket basename found in XDG_RUNTIME_DIR")
				return cand, ""
			}
		}
		if !filepath.IsAbs(raw) {
			cand := filepath.Join(runtime, raw)
			if usableSocket(cand) {
				debug("editor socket found in XDG_RUNTIME_DIR")
				return cand, ""
			}
		}
		matches, _ := filepath.Glob(filepath.Join(runtime, "vscode-git-*.sock"))
		var socks []string
		for _, m := range matches {
			if usableSocket(m) {
				socks = append(socks, m)
			}
		}
		switch len(socks) {
		case 1:
			debug("using the only vscode-git socket in XDG_RUNTIME_DIR")
			return socks[0], ""
		case 0:
		default:
			return "", "editor socket not found (several vscode-git sockets in XDG_RUNTIME_DIR)"
		}
	}
	if _, err := os.Stat(raw); err != nil {
		return "", "editor socket not found"
	}
	return "", "editor socket not found"
}

func usableSocket(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return false
	}
	return fi.Mode()&os.ModeSocket != 0
}

// probeEditorSocket dials and closes. It does not POST, so no dialog is shown.
func probeEditorSocket(path string) error {
	conn, err := dialEditor(path)
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}

func askEditor(socketPath, prompt string) ([]byte, error) {
	if !usableSocket(socketPath) {
		return nil, &editorSkip{reason: "editor socket not found"}
	}

	conn, err := dialEditor(socketPath)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(os.Stderr, "Asking for the passphrase with the editor prompt\n")
	debug("using the editor prompt")

	placeholder := editorPlaceholder(prompt)
	token := os.Getenv("VSCODE_GIT_IPC_AUTH_TOKEN")
	payload, err := editorPayload(placeholder, token)
	if err != nil {
		conn.Close()
		return nil, &editorSkip{reason: "editor prompt failed"}
	}

	status, raw, err := postAskpass(conn, payload, token)
	securemem.Wipe(payload)
	if err != nil {
		if errors.Is(err, errTooLongBody) {
			return nil, ErrTooLong
		}
		if isTimeoutErr(err) {
			return nil, &editorSkip{reason: "editor socket timed out"}
		}
		return nil, &editorSkip{reason: "editor prompt failed"}
	}
	if status != 200 {
		securemem.Wipe(raw)
		return nil, &editorSkip{reason: fmt.Sprintf("editor prompt returned HTTP %d", status)}
	}
	return decodeEditorBody(raw)
}

func editorPayload(placeholder, token string) ([]byte, error) {
	if token != "" && !strings.ContainsAny(token, "\r\n") {
		return json.Marshal(struct {
			AskpassType string `json:"askpassType"`
			Request     string `json:"request"`
			Host        string `json:"host"`
		}{
			AskpassType: "https",
			Request:     placeholder,
			Host:        editorHost,
		})
	}
	return json.Marshal(struct {
		AskpassType string   `json:"askpassType"`
		Argv        []string `json:"argv"`
	}{
		AskpassType: "https",
		// askpass.sh word-splits the Git prompt, so the workbench reads
		// the placeholder from argv[2] and the URL from argv[4].
		Argv: []string{"pogo", "askpass", placeholder, "for", editorHost},
	})
}

func dialEditor(socketPath string) (net.Conn, error) {
	d := net.Dialer{Timeout: dialTimeout}
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()
	conn, err := d.DialContext(ctx, "unix", socketPath)
	if err != nil {
		if isTimeoutErr(err) {
			return nil, &editorSkip{reason: "editor socket timed out"}
		}
		if isConnRefused(err) {
			return nil, &editorSkip{reason: "editor socket connection refused"}
		}
		return nil, &editorSkip{reason: "editor socket connection refused"}
	}
	return conn, nil
}

func postAskpass(conn net.Conn, payload []byte, token string) (status int, body []byte, err error) {
	defer conn.Close()
	deadline := time.Now().Add(promptTimeout)
	if err := conn.SetDeadline(deadline); err != nil {
		return 0, nil, err
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "POST /askpass HTTP/1.1\r\nHost: unix\r\nContent-Type: application/json\r\nContent-Length: %d\r\n", len(payload))
	if token != "" && !strings.ContainsAny(token, "\r\n") {
		fmt.Fprintf(&buf, "x-cursor-git-auth-token: %s\r\n", token)
	}
	buf.WriteString("Connection: close\r\n\r\n")
	buf.Write(payload)
	if _, err := conn.Write(buf.Bytes()); err != nil {
		return 0, nil, err
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	// A JSON string can escape each byte as \u00XX. Bound the read so a
	// huge body is wiped, while a MaxBytes passphrase still fits.
	limit := int64(MaxBytes)*6 + 64
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		securemem.Wipe(raw)
		return resp.StatusCode, nil, err
	}
	if int64(len(raw)) > limit {
		securemem.Wipe(raw)
		return resp.StatusCode, nil, errTooLongBody
	}
	return resp.StatusCode, raw, nil
}

var errTooLongBody = errors.New("editor response too long")

func decodeEditorBody(raw []byte) ([]byte, error) {
	defer securemem.Wipe(raw)
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, ErrCancelled
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, &editorSkip{reason: "editor prompt returned an unreadable response"}
	}
	rawPass := []byte(s)
	pass := stripOneNewline(rawPass)
	if len(pass) > MaxBytes {
		securemem.Wipe(rawPass)
		return nil, ErrTooLong
	}
	if len(pass) == 0 {
		securemem.Wipe(rawPass)
		return nil, ErrCancelled
	}
	out := make([]byte, len(pass))
	copy(out, pass)
	securemem.Wipe(rawPass)
	return out, nil
}

func isTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded")
}

func isConnRefused(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "connection refused")
}
