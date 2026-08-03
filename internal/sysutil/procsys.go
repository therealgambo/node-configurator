package sysutil

import (
	"fmt"
	"os"
	"strings"
)

// SysctlPath translates a dotted sysctl key ("net.core.somaxconn") into its
// /proc/sys file path, rooted at e.Root. "/" and ".." are stripped
// defensively so a malformed or attacker-influenced key can't escape
// /proc/sys.
func (e *Env) SysctlPath(key string) string {
	key = strings.ReplaceAll(key, "..", "")
	key = strings.ReplaceAll(key, "/", "")
	elem := strings.Split(key, ".")
	pathElems := append([]string{"proc", "sys"}, elem...)
	return e.Path(pathElems...)
}

// ReadSysctl reads the current value of a sysctl key. The returned value has
// trailing whitespace/newlines trimmed.
func (e *Env) ReadSysctl(key string) (string, error) {
	data, err := os.ReadFile(e.SysctlPath(key))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// WriteSysctl writes value to the sysctl key's /proc/sys file. It does not
// check the current value first — callers that want idempotency/diffing
// should use ReadSysctl to compare beforehand (see resource.SysctlResource).
func (e *Env) WriteSysctl(key, value string) error {
	path := e.SysctlPath(key)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("sysctl %s not available on this kernel: %w", key, err)
	}
	return os.WriteFile(path, []byte(value), 0o644)
}

// SysctlExists reports whether the given sysctl key exists on this kernel.
func (e *Env) SysctlExists(key string) bool {
	_, err := os.Stat(e.SysctlPath(key))
	return err == nil
}
