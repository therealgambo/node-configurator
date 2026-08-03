package resource

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/therealgambo/node-configurator/internal/sysutil"
)

// SysctlResource converges a bundle of sysctl key/value pairs against the
// live kernel and persists them to a sysctl.d drop-in file so they survive
// reboots independent of this tool's own systemd unit re-running.
//
// Keys that don't exist on the running kernel (e.g. a feature gated behind a
// kernel version or config not present in this distro's build) are skipped
// rather than treated as failures, since the desired-state map is shared
// across a fleet of varying instance families/kernels.
type SysctlResource struct {
	Env         *sysutil.Env
	Settings    map[string]string
	PersistFile string // e.g. /etc/sysctl.d/99-node-configurator.conf; empty disables persistence
}

func (r *SysctlResource) Name() string { return "sysctl" }

func (r *SysctlResource) Check(ctx context.Context) (Result, error) {
	drift, skipped, err := r.diffLive()
	if err != nil {
		return Result{Name: r.Name(), Status: StatusFailed, Detail: err.Error()}, err
	}

	persistDrift := r.persistNeeded()

	if len(drift) == 0 && !persistDrift {
		detail := ""
		if len(skipped) > 0 {
			detail = fmt.Sprintf("%d key(s) not present on this kernel, skipped: %s", len(skipped), strings.Join(skipped, ", "))
		}
		return Result{Name: r.Name(), Status: StatusUnchanged, Detail: detail}, nil
	}

	detail := strings.Join(drift, ", ")
	if persistDrift {
		detail = joinDetails(detail, "drop-in file out of date")
	}
	return Result{Name: r.Name(), Status: StatusWouldChange, Detail: detail}, nil
}

func (r *SysctlResource) Apply(ctx context.Context) error {
	for _, key := range sortedKeys(r.Settings) {
		want := r.Settings[key]
		if !r.Env.SysctlExists(key) {
			continue
		}
		cur, err := r.Env.ReadSysctl(key)
		if err == nil && normalizeSysctlValue(cur) == normalizeSysctlValue(want) {
			continue
		}
		if err := r.Env.WriteSysctl(key, want); err != nil {
			return fmt.Errorf("applying sysctl %s=%s: %w", key, want, err)
		}
	}

	if r.PersistFile != "" {
		if _, err := sysutil.WriteFileIfChanged(r.PersistFile, []byte(renderSysctlFile(r.Settings)), 0o644); err != nil {
			return fmt.Errorf("persisting sysctl drop-in %s: %w", r.PersistFile, err)
		}
	}
	return nil
}

func (r *SysctlResource) diffLive() (drift, skipped []string, err error) {
	for _, key := range sortedKeys(r.Settings) {
		want := r.Settings[key]
		if !r.Env.SysctlExists(key) {
			skipped = append(skipped, key)
			continue
		}
		cur, readErr := r.Env.ReadSysctl(key)
		if readErr != nil {
			return nil, nil, fmt.Errorf("reading sysctl %s: %w", key, readErr)
		}
		if normalizeSysctlValue(cur) != normalizeSysctlValue(want) {
			drift = append(drift, fmt.Sprintf("%s: %q -> %q", key, cur, want))
		}
	}
	return drift, skipped, nil
}

// normalizeSysctlValue collapses whitespace runs so multi-value sysctls
// compare correctly against what the kernel echoes back. Found live: the
// kernel always renders e.g. net.ipv4.tcp_rmem's three fields tab-separated
// on read regardless of how they were written (we write them
// space-separated), so a byte-exact comparison would report permanent
// drift immediately after a successful apply.
func normalizeSysctlValue(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func (r *SysctlResource) persistNeeded() bool {
	if r.PersistFile == "" {
		return false
	}
	existing, err := sysutil.ReadFileString(r.PersistFile)
	if err != nil {
		return true
	}
	return existing != renderSysctlFile(r.Settings)
}

func renderSysctlFile(settings map[string]string) string {
	var b strings.Builder
	b.WriteString("# Managed by node-configurator. Local edits will be overwritten.\n")
	for _, key := range sortedKeys(settings) {
		fmt.Fprintf(&b, "%s = %s\n", key, settings[key])
	}
	return b.String()
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
