package engine

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/therealgambo/node-configurator/internal/resource"
)

// ExitCode maps a set of Results to a process exit code, following the
// Puppet/Ansible check-mode convention: 0 = nothing to do (or everything
// converged successfully), 1 = a resource failed, 2 = ModeCheck found
// pending changes it did not make.
func ExitCode(results []resource.Result, mode Mode) int {
	failed := false
	changed := false
	for _, r := range results {
		if r.Status == resource.StatusFailed {
			failed = true
		}
		if r.Status.Changed() {
			changed = true
		}
	}
	switch {
	case failed:
		return 1
	case mode == ModeCheck && changed:
		return 2
	default:
		return 0
	}
}

// WriteTextReport renders results as a human-readable table, one line per
// resource.
func WriteTextReport(w io.Writer, results []resource.Result) error {
	for _, r := range results {
		if _, err := fmt.Fprintln(w, r.String()); err != nil {
			return err
		}
	}
	return nil
}

// WriteJSONReport renders results as an indented JSON array, for machine
// consumption (log aggregation, CI/health checks).
func WriteJSONReport(w io.Writer, results []resource.Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}
