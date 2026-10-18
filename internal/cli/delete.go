package cli

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"pogo/internal/securemem"
)

// notAWipe is the note printed when convert removes a plaintext file and
// when it tells the person how to remove one later. os.Remove is not a secure wipe.
const notAWipe = "This is a normal delete, not a secure wipe. Copies can remain on disk."

// deleteAnswerReader opens the place convert reads the delete answer.
// The controlling terminal comes first, so a --key-stdin pipe can carry the
// passphrase while the question still reaches the person. When that terminal
// is missing, stdin is used only if it is itself a terminal. Tests replace this.
var deleteAnswerReader = openDeleteAnswerReader

func openDeleteAnswerReader() (io.ReadWriteCloser, error) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err == nil {
		if term.IsTerminal(int(f.Fd())) {
			return f, nil
		}
		f.Close()
	}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		return stdinTerminal{os.Stdin}, nil
	}
	if err != nil {
		return nil, err
	}
	return nil, errors.New("no controlling terminal")
}

// stdinTerminal reads and writes the process stdin without closing it.
// Closing the person's standard input would break the rest of the session.
type stdinTerminal struct{ *os.File }

func (stdinTerminal) Close() error { return nil }

// answeredYes reports whether the person accepted the default-no question.
// Only "y" and "yes" count. Comparison ignores case and surrounding space.
func answeredYes(line string) bool {
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

func deletePrompt(plainPath string) string {
	return "Delete the original " + quotePath(plainPath) + "? [y/N] "
}

// askDeleteOriginal asks on the controlling terminal.
// No terminal leaves asked false so the caller can explain why it did not ask.
// Enter, any answer other than y or yes, and EOF (including a partial line
// with no newline) keep the file.
func askDeleteOriginal(plainPath string) (yes bool, asked bool) {
	rw, err := deleteAnswerReader()
	if err != nil {
		return false, false
	}
	defer rw.Close()
	if _, err := io.WriteString(rw, deletePrompt(plainPath)); err != nil {
		fmt.Fprintf(os.Stderr, "Could not ask whether to delete %s (%s).\n", plainPath, err.Error())
		return false, false
	}
	line, err := bufio.NewReader(rw).ReadString('\n')
	if errors.Is(err, io.EOF) {
		return false, true
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not read the answer (%s).\n", err.Error())
		return false, true
	}
	return answeredYes(line), true
}

type keepReason int

const (
	keepNoTerminal keepReason = iota
	keepAnswer
	keepFlag
)

func reportKept(plainPath string, why keepReason) {
	fmt.Fprintf(os.Stderr, "Left the plaintext file in place: %s\n", plainPath)
	switch why {
	case keepFlag, keepAnswer:
		fmt.Fprintf(os.Stderr, "Delete it yourself when you are ready:\n  rm %s\n", quotePath(plainPath))
	default:
		fmt.Fprintf(os.Stderr, "No terminal, so Pogo did not ask. The plaintext file was kept.\nDelete it yourself, or pass --delete-original:\n  rm %s\n", quotePath(plainPath))
	}
	fmt.Fprintf(os.Stderr, "%s\n", notAWipe)
}

func removePlaintext(path string) error {
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("Could not delete %s: %s.\nThe encrypted file is in place. The plaintext file was left where it is.\nDelete it yourself with rm. %s", path, err.Error(), notAWipe)
	}
	return nil
}

func finishConvert(opts options, plainPath, enc string) (int, error) {
	fmt.Fprintf(os.Stderr, "Wrote %s\nChecked that %s decrypts.\n", enc, enc)
	remove := false
	why := keepNoTerminal
	switch {
	case opts.deleteOriginal:
		remove = true
	case opts.keepOriginal:
		why = keepFlag
	default:
		yes, asked := askDeleteOriginal(plainPath)
		if yes {
			remove = true
		} else if asked {
			why = keepAnswer
		}
	}
	if !remove {
		reportKept(plainPath, why)
		return 0, nil
	}
	if err := removePlaintext(plainPath); err != nil {
		return 1, err
	}
	fmt.Fprintf(os.Stderr, "Removed %s\n%s\n", plainPath, notAWipe)
	return 0, nil
}

// verifySealed reads the encrypted file back and checks the plaintext.
// Convert calls it before offering to delete. Tests replace it.
var verifySealed = verifyRoundTrip

func verifyRoundTrip(path string, pass, plain []byte) error {
	got, err := decryptFile(path, pass)
	if err != nil {
		return fmt.Errorf("Wrote %s, but decrypting it failed: %s.\nThe plaintext file was left in place.", path, err.Error())
	}
	defer securemem.Wipe(got)
	if !bytes.Equal(got, plain) {
		return fmt.Errorf("Wrote %s, but the decrypted text did not match the plaintext.\nThe plaintext file was left in place.", path)
	}
	return nil
}
