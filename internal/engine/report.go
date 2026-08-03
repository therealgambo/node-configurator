package engine

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/therealgambo/node-configurator/internal/facts"
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

// WriteFactsSummary renders the instance/host facts a run is based on, so
// there's a record of the pre-change state in the log even if a later
// resource fails midway through converging. It's written before any
// resource is evaluated, not just before the final report.
func WriteFactsSummary(w io.Writer, f facts.Facts) error {
	var b strings.Builder

	fmt.Fprintf(&b, "=== Discovered facts (source: %s) ===\n", f.Source)
	if f.InstanceType != "" {
		fmt.Fprintf(&b, "Instance:  %s (family %s, %s", f.InstanceType, f.Family, f.Arch)
		if f.Hypervisor != "" {
			fmt.Fprintf(&b, ", %s hypervisor", f.Hypervisor)
		}
		b.WriteString(")\n")
	} else {
		fmt.Fprintf(&b, "Instance:  unknown (arch %s)\n", f.Arch)
	}
	if f.Region != "" {
		fmt.Fprintf(&b, "Region:    %s (az %s)\n", f.Region, f.AvailabilityZone)
	}
	fmt.Fprintf(&b, "Sizing:    %d vCPUs, %d MiB memory, ~%v Gbps network", f.VCPUs, f.MemMiB, f.NetworkBandwidthGbps)
	if f.EnaSupport {
		b.WriteString(" (ENA)")
	}
	if f.MaxENIs > 0 {
		fmt.Fprintf(&b, ", maxENIs=%d", f.MaxENIs)
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "Storage:   NVMe instance store=%t, bare metal=%t\n", f.NvmeInstanceStore, f.BareMetal)
	if f.Distro.ID != "" || f.KernelVersion != "" {
		fmt.Fprintf(&b, "Host:      %s %s, kernel %s\n", f.Distro.ID, f.Distro.VersionID, f.KernelVersion)
	}

	_, err := io.WriteString(w, b.String())
	return err
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

// Report is the --log-format json shape: the facts a run was based on plus
// the outcome of every resource, as a single parseable document (rather
// than facts and results being two separate JSON values in the stream).
type Report struct {
	Facts   facts.Facts       `json:"facts"`
	Results []resource.Result `json:"results"`
}

// WriteJSONReport renders an indented Report document, for machine
// consumption (log aggregation, CI/health checks).
func WriteJSONReport(w io.Writer, f facts.Facts, results []resource.Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(Report{Facts: f, Results: results})
}
