package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"pogo/internal/filefmt"
)

func readLimited(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	if st.Size() > max {
		return nil, fmt.Errorf("%s is %d bytes, which is larger than the %d byte limit", path, st.Size(), max)
	}
	return io.ReadAll(io.LimitReader(f, max+1))
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	f, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return fmt.Errorf("could not create a temporary file next to %s: %w", path, err)
	}
	tmp := f.Name()
	cleanup := true
	defer func() {
		if cleanup {
			f.Close()
			os.Remove(tmp)
		}
	}()
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("could not replace %s: %w", path, err)
	}
	cleanup = false
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		d.Close()
	}
	return nil
}

func fileExists(path string) (bool, error) {
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if st.IsDir() {
		return true, fmt.Errorf("%s is a directory, not a file", path)
	}
	return true, nil
}

func refuseOverwrite(path string) error {
	return fmt.Errorf("%s already exists, so Pogo did not overwrite it.\nChoose a different -f path, or pass --force to replace it. Replacing deletes the previous contents.", path)
}

func missingEnc(path string) error {
	if path == ".env.enc" {
		return fmt.Errorf("No .env.enc found in this folder.\nCreate one with:\n  pogo init\nOr convert an existing file with:\n  pogo convert .env")
	}
	return fmt.Errorf("No encrypted file at %s.\nCreate one with:\n  pogo init -f %s\nOr convert an existing file with:\n  pogo convert .env -f %s", path, path, path)
}

const maxCipher = filefmt.MaxPlaintext + 4096
