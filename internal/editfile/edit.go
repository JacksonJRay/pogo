// Package editfile decrypts into a private temporary file, runs the user's
// editor, and securely deletes the temporary file afterwards.
package editfile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/term"

	"pogo/internal/securemem"
)

const maxPlain = 32 << 20

var (
	// ErrEditor means the editor process ran and exited non-zero.
	ErrEditor = errors.New("editor exited non-zero")
	// ErrEditorMissing means the editor program could not be started.
	ErrEditorMissing = errors.New("editor not found")
	// ErrInterrupted means Pogo received a signal and aborted the edit.
	ErrInterrupted = errors.New("interrupted")
)

// EditorCommand returns the editor a person asked for.
// $EDITOR wins. If it is unset or blank, $VISUAL is used. If both are unset, vim.
func EditorCommand() string {
	if e := strings.TrimSpace(os.Getenv("EDITOR")); e != "" {
		return e
	}
	if e := strings.TrimSpace(os.Getenv("VISUAL")); e != "" {
		return e
	}
	return "vim"
}

// TempBase prefers a memory-backed directory so the plaintext is less likely
// to be written to a physical disk. On macOS /dev/shm does not exist and the
// process temporary directory is used instead.
func TempBase() string {
	if writableDir("/dev/shm") {
		return "/dev/shm"
	}
	return os.TempDir()
}

func writableDir(dir string) bool {
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return false
	}
	f, err := os.CreateTemp(dir, ".pogo-w-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

// Session is the result of creating the temporary file, before the editor runs.
// Tests use it to check permissions. Callers normally use Edit.
type Session struct {
	Dir  string
	Path string
}

// Edit writes initial to a 0600 file in a 0700 directory, runs editorCmd with
// the file path appended as the last argument, and returns the file contents
// when the editor exits 0. The temporary file is overwritten and removed on
// every return path, including editor failure and signals.
//
// editorCmd is split on whitespace into a program and arguments. It is not
// passed to a shell.
func Edit(initial []byte, editorCmd string) ([]byte, error) {
	if len(initial) > maxPlain {
		return nil, fmt.Errorf("plaintext is larger than the %d byte limit", maxPlain)
	}
	fields := strings.Fields(editorCmd)
	if len(fields) == 0 {
		return nil, fmt.Errorf("%w: editor command is empty", ErrEditorMissing)
	}
	dir, err := os.MkdirTemp(TempBase(), "pogo-")
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	path := dir + "/env"
	cleanup := func() {
		_ = SecureRemove(path)
		_ = os.RemoveAll(dir)
	}
	defer cleanup()

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(initial); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, err
	}

	cmd, foreground, parentPgrp, err := startEditor(fields, path)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrEditorMissing, fields[0])
		}
		return nil, fmt.Errorf("%w: %s", ErrEditorMissing, err.Error())
	}
	if foreground {
		defer restoreForeground(int(os.Stdin.Fd()), parentPgrp)
	}
	pgid, pgErr := syscall.Getpgid(cmd.Process.Pid)
	if pgErr != nil {
		pgid = cmd.Process.Pid
	}

	var signaled atomic.Bool
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	var once sync.Once
	stop := func() { once.Do(func() { signal.Stop(sigCh) }) }
	defer stop()
	done := make(chan struct{})
	abortKill := make(chan struct{})
	go func() {
		select {
		case <-sigCh:
			// The editor is the foreground program, so a terminal Ctrl-C hits
			// the editor, not Pogo. A signal delivered to Pogo means abort:
			// TERM, then KILL, because vim treats SIGINT as "cancel this command"
			// and would otherwise keep the temp file open.
			signaled.Store(true)
			_ = syscall.Kill(-pgid, syscall.SIGTERM)
			select {
			case <-time.After(400 * time.Millisecond):
				_ = syscall.Kill(-pgid, syscall.SIGKILL)
			case <-abortKill:
			}
		case <-done:
		}
	}()
	waitErr := cmd.Wait()
	close(done)
	close(abortKill)
	stop()

	if signaled.Load() {
		return nil, ErrInterrupted
	}
	if waitErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrEditor, waitErr)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(out) > maxPlain {
		securemem.Wipe(out)
		return nil, fmt.Errorf("edited file is larger than the %d byte limit", maxPlain)
	}
	return out, nil
}

// startEditor runs the editor with the temp path as the last argument.
// On a terminal, the editor becomes the foreground process group so vim and
// nano can read keystrokes. If that fails, it starts again in its own group.
func startEditor(fields []string, path string) (cmd *exec.Cmd, foreground bool, parentPgrp int, err error) {
	fd := int(os.Stdin.Fd())
	foreground = term.IsTerminal(fd)
	if foreground {
		parentPgrp = syscall.Getpgrp()
	}
	cmd = editorCmd(fields, path, foreground, fd)
	if err = cmd.Start(); err != nil && foreground {
		foreground = false
		parentPgrp = 0
		cmd = editorCmd(fields, path, false, fd)
		err = cmd.Start()
	}
	return cmd, foreground, parentPgrp, err
}

func editorCmd(fields []string, path string, foreground bool, fd int) *exec.Cmd {
	cmd := exec.Command(fields[0], append(append([]string{}, fields[1:]...), path)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if foreground {
		cmd.SysProcAttr = &syscall.SysProcAttr{Foreground: true, Ctty: fd}
	} else {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	return cmd
}

func restoreForeground(fd, pgrp int) {
	if fd < 0 || pgrp <= 0 {
		return
	}
	signal.Ignore(syscall.SIGTTOU)
	defer signal.Reset(syscall.SIGTTOU)
	pg := int32(pgrp)
	_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TIOCSPGRP), uintptr(unsafe.Pointer(&pg)))
}

// SecureRemove overwrites path with random bytes, then zeros, then deletes it.
// A missing file is not an error.
func SecureRemove(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		_ = os.Remove(path)
		return err
	}
	size := st.Size()
	buf := make([]byte, 64*1024)
	if _, err := io.ReadFull(randReader{}, buf); err != nil && size > 0 {
		// Fall back to a zero pass only.
		for i := range buf {
			buf[i] = 0
		}
	}
	writePass := func() error {
		if _, err := f.Seek(0, 0); err != nil {
			return err
		}
		var written int64
		for written < size {
			n := int64(len(buf))
			if remain := size - written; remain < n {
				n = remain
			}
			if _, err := f.Write(buf[:n]); err != nil {
				return err
			}
			written += n
		}
		return nil
	}
	err = writePass()
	for i := range buf {
		buf[i] = 0
	}
	if err == nil {
		err = writePass()
	}
	syncErr := f.Sync()
	closeErr := f.Close()
	rmErr := os.Remove(path)
	securemem.Wipe(buf)
	if err != nil {
		return err
	}
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	return rmErr
}

// randReader adapts crypto/rand without importing it at the call site name.
type randReader struct{}

func (randReader) Read(p []byte) (int, error) {
	return readRand(p)
}
