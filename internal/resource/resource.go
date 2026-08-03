// Package resource defines the idempotent unit of work the engine converges:
// each Resource knows how to Check current-vs-desired state and Apply the
// desired state. Resources never assume they are the only thing that ran —
// Check must always re-derive truth from the live system rather than trust
// in-memory state from a previous call.
package resource

import (
	"context"
	"fmt"
	"strings"
)

// Status is the outcome of checking or applying a Resource.
type Status string

const (
	// StatusUnchanged means the live system already matches desired state.
	StatusUnchanged Status = "unchanged"
	// StatusWouldChange means Check found drift but Apply was not called
	// (dry-run / `check` subcommand).
	StatusWouldChange Status = "would_change"
	// StatusChanged means Apply successfully converged drift.
	StatusChanged Status = "changed"
	// StatusFailed means Check or Apply hit an unexpected error.
	StatusFailed Status = "failed"
	// StatusSkipped means the resource does not apply to this host (e.g. a
	// sysfs knob absent on this kernel/architecture) and was intentionally
	// left alone.
	StatusSkipped Status = "skipped"
	// StatusRebootRequired means desired state cannot be reached without a
	// reboot (e.g. cgroup v1->v2, 1G hugepages). The resource does not edit
	// the bootloader or reboot the node; it only reports the condition.
	StatusRebootRequired Status = "reboot_required"
)

// Changed reports whether the status represents a live or pending mutation.
func (s Status) Changed() bool {
	return s == StatusChanged || s == StatusWouldChange
}

// Result describes the outcome of checking or applying one Resource.
type Result struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	// Detail is a short human-readable explanation: what differed, what was
	// skipped and why, or the error that occurred.
	Detail string `json:"detail,omitempty"`
}

func (r Result) String() string {
	if r.Detail == "" {
		return fmt.Sprintf("%-28s %s", r.Name, r.Status)
	}
	return fmt.Sprintf("%-28s %-16s %s", r.Name, r.Status, r.Detail)
}

// Resource is one idempotent, named unit of desired-state convergence.
type Resource interface {
	// Name uniquely identifies the resource for reporting and --only/--skip
	// filtering, e.g. "sysctl", "swap", "irq-affinity".
	Name() string
	// Check compares live state to desired state without mutating anything.
	// A non-nil error means the check itself failed (e.g. unreadable file);
	// drift that is expected and by-design (missing optional kernel feature)
	// should be reported via StatusSkipped in Result, not an error.
	Check(ctx context.Context) (Result, error)
	// Apply converges live state to desired state. It is only called by the
	// engine when Check reported drift. Implementations should still be
	// safe to call unconditionally (idempotent) since callers may do so.
	Apply(ctx context.Context) error
}

// joinDetails joins non-empty detail fragments with "; " for a compact
// single-line Result.Detail.
func joinDetails(fragments ...string) string {
	nonEmpty := fragments[:0:0]
	for _, f := range fragments {
		if f != "" {
			nonEmpty = append(nonEmpty, f)
		}
	}
	return strings.Join(nonEmpty, "; ")
}
