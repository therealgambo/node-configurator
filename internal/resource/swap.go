package resource

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/therealgambo/node-configurator/internal/sysutil"
)

// SwapResource enforces swap being fully disabled: turns off any active
// swap, comments out swap entries in /etc/fstab, and masks any discovered
// systemd .swap units so they can't turn it back on at the next boot.
// Kubernetes nodes in this fleet run without swap (NodeSwap is a non-goal).
type SwapResource struct {
	Env    *sysutil.Env
	Runner sysutil.Runner
}

func (r *SwapResource) Name() string { return "swap" }

func (r *SwapResource) Check(ctx context.Context) (Result, error) {
	active, err := r.activeSwapDevices()
	if err != nil {
		return Result{Name: r.Name(), Status: StatusFailed, Detail: err.Error()}, err
	}

	fstabContent, _ := sysutil.ReadFileString(r.fstabPath())
	fstabDrift := fstabHasActiveSwapLines(fstabContent)

	units := r.activeSwapUnits(ctx)

	if len(active) == 0 && !fstabDrift && len(units) == 0 {
		return Result{Name: r.Name(), Status: StatusUnchanged}, nil
	}

	detail := ""
	if len(active) > 0 {
		detail = fmt.Sprintf("active swap: %s", strings.Join(active, ", "))
	}
	if fstabDrift {
		detail = joinDetails(detail, "/etc/fstab has active swap entries")
	}
	if len(units) > 0 {
		detail = joinDetails(detail, fmt.Sprintf("active swap units: %s", strings.Join(units, ", ")))
	}
	return Result{Name: r.Name(), Status: StatusWouldChange, Detail: detail}, nil
}

func (r *SwapResource) Apply(ctx context.Context) error {
	active, err := r.activeSwapDevices()
	if err != nil {
		return err
	}
	if len(active) > 0 {
		if _, err := r.Runner.Run(ctx, "swapoff", "-a"); err != nil {
			return fmt.Errorf("swapoff -a: %w", err)
		}
	}

	if err := r.commentFstabSwapLines(); err != nil {
		return err
	}

	units := r.activeSwapUnits(ctx)
	for _, unit := range units {
		if _, err := r.Runner.Run(ctx, "systemctl", "mask", unit); err != nil {
			return fmt.Errorf("systemctl mask %s: %w", unit, err)
		}
	}
	return nil
}

func (r *SwapResource) activeSwapDevices() ([]string, error) {
	f, err := os.Open(r.Env.Path("proc", "swaps"))
	if err != nil {
		return nil, fmt.Errorf("reading /proc/swaps: %w", err)
	}
	defer func() { _ = f.Close() }()

	var devices []string
	scanner := bufio.NewScanner(f)
	first := true
	for scanner.Scan() {
		if first {
			first = false // header line: "Filename  Type  Size  Used  Priority"
			continue
		}
		fields := strings.Fields(scanner.Text())
		if len(fields) > 0 {
			devices = append(devices, fields[0])
		}
	}
	return devices, scanner.Err()
}

// activeSwapUnits queries systemd for active .swap units. systemctl being
// unavailable (minimal containers, non-systemd test environments) is
// treated as "no units to mask", not a failure.
func (r *SwapResource) activeSwapUnits(ctx context.Context) []string {
	out, err := r.Runner.Run(ctx, "systemctl", "list-units", "--type=swap", "--no-legend", "--plain")
	if err != nil {
		return nil
	}
	var units []string
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) > 0 {
			units = append(units, fields[0])
		}
	}
	return units
}

func (r *SwapResource) fstabPath() string {
	return r.Env.Path("etc", "fstab")
}

// fstabHasActiveSwapLines reports whether fstab content has a non-comment
// entry whose filesystem-type field is "swap".
func fstabHasActiveSwapLines(fstabContent string) bool {
	for line := range strings.SplitSeq(fstabContent, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) >= 3 && fields[2] == "swap" {
			return true
		}
	}
	return false
}

func (r *SwapResource) commentFstabSwapLines() error {
	content, err := sysutil.ReadFileString(r.fstabPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading /etc/fstab: %w", err)
	}
	if !fstabHasActiveSwapLines(content) {
		return nil
	}

	if err := sysutil.BackupFile(r.fstabPath()); err != nil {
		return fmt.Errorf("backing up /etc/fstab: %w", err)
	}

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) >= 3 && fields[2] == "swap" {
			lines[i] = "# disabled by node-configurator: " + line
		}
	}

	if _, err := sysutil.WriteFileIfChanged(r.fstabPath(), []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return fmt.Errorf("writing /etc/fstab: %w", err)
	}
	return nil
}
