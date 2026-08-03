// Package sysutil provides low-level OS primitives (file I/O, /proc/sys
// access, command execution, distro detection) used by the resource and
// facts packages. Every primitive is rooted at an Env.Root path (default
// "/") so tests can point it at a scratch directory instead of the real
// filesystem.
package sysutil

import "path/filepath"

// Env carries the filesystem root all sysutil operations are relative to.
// Production code uses the zero value (Root == "" behaves as "/"); tests
// set Root to a t.TempDir() populated with fake /proc, /sys, /etc files.
type Env struct {
	Root string
}

// New returns an Env rooted at the real filesystem.
func New() *Env {
	return &Env{Root: "/"}
}

// Path joins elem onto the Env's root.
func (e *Env) Path(elem ...string) string {
	root := e.Root
	if root == "" {
		root = "/"
	}
	parts := append([]string{root}, elem...)
	return filepath.Join(parts...)
}
