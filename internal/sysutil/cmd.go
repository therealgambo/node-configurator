package sysutil

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// DefaultCmdTimeout bounds external command execution (modprobe, ethtool,
// ip, swapoff) so a hung command can't stall the whole apply run.
const DefaultCmdTimeout = 15 * time.Second

// Runner executes external commands. It is an interface so resource tests
// can substitute a fake instead of shelling out to modprobe/ethtool/etc.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (stdout string, err error)
}

// ExecRunner is the production Runner backed by os/exec.
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultCmdTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("%s %v: %w (stderr: %s)", name, args, err, stderr.String())
	}
	return stdout.String(), nil
}
