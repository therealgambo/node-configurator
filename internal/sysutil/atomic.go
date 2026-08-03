package sysutil

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// ReadFileString reads a file and returns its contents as a string,
// returning the error unchanged (including os.ErrNotExist) for callers to
// inspect with os.IsNotExist.
func ReadFileString(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteFileIfChanged writes content to path with the given mode only if the
// file doesn't already exist with identical content. It reports whether a
// write occurred. The write is atomic (write to a temp file, then rename).
func WriteFileIfChanged(path string, content []byte, mode os.FileMode) (changed bool, err error) {
	existing, err := os.ReadFile(path)
	if err == nil && bytes.Equal(existing, content) {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("reading %s: %w", path, err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("creating parent dir for %s: %w", path, err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return false, fmt.Errorf("creating temp file for %s: %w", path, err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }() // no-op once renamed

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return false, fmt.Errorf("writing temp file for %s: %w", path, err)
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return false, fmt.Errorf("chmod temp file for %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return false, fmt.Errorf("closing temp file for %s: %w", path, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return false, fmt.Errorf("renaming temp file into %s: %w", path, err)
	}
	return true, nil
}

// BackupFile copies path to a timestamped sibling (path + ".node-configurator.bak-<unix>")
// the first time it is modified. It is a no-op if path does not exist or a
// backup already exists.
func BackupFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading %s for backup: %w", path, err)
	}

	backupPath := fmt.Sprintf("%s.node-configurator.orig", path)
	if _, err := os.Stat(backupPath); err == nil {
		return nil // already backed up once
	}

	return os.WriteFile(backupPath, data, 0o644)
}
