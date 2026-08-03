package resource

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/therealgambo/node-configurator/internal/sysutil"
)

// CPUGovernorResource sets the cpufreq scaling governor on every CPU, e.g.
// to "performance" to avoid frequency-scaling jitter for latency-sensitive
// containerized workloads. Some instance types/architectures (notably some
// Graviton configurations) don't expose an adjustable governor at all —
// that's reported as Skipped, not a failure.
type CPUGovernorResource struct {
	Env      *sysutil.Env
	Governor string // e.g. "performance"; empty disables this resource
}

func (r *CPUGovernorResource) Name() string { return "cpu-governor" }

func (r *CPUGovernorResource) Check(ctx context.Context) (Result, error) {
	if r.Governor == "" {
		return Result{Name: r.Name(), Status: StatusSkipped, Detail: "disabled"}, nil
	}

	paths, err := r.governorPaths()
	if err != nil {
		return Result{Name: r.Name(), Status: StatusFailed, Detail: err.Error()}, err
	}
	if len(paths) == 0 {
		return Result{Name: r.Name(), Status: StatusSkipped, Detail: "no adjustable cpufreq governor on this host"}, nil
	}

	var drift int
	for _, p := range paths {
		cur, err := sysutil.ReadFileString(p)
		if err != nil {
			continue
		}
		if strings.TrimSpace(cur) != r.Governor {
			drift++
		}
	}

	if drift == 0 {
		return Result{Name: r.Name(), Status: StatusUnchanged}, nil
	}
	return Result{Name: r.Name(), Status: StatusWouldChange, Detail: fmt.Sprintf("%d of %d CPUs not on %q governor", drift, len(paths), r.Governor)}, nil
}

func (r *CPUGovernorResource) Apply(ctx context.Context) error {
	if r.Governor == "" {
		return nil
	}
	paths, err := r.governorPaths()
	if err != nil {
		return err
	}
	for _, p := range paths {
		cur, err := sysutil.ReadFileString(p)
		if err == nil && strings.TrimSpace(cur) == r.Governor {
			continue
		}
		if err := os.WriteFile(p, []byte(r.Governor), 0o644); err != nil {
			return fmt.Errorf("setting governor via %s: %w", p, err)
		}
	}
	return nil
}

func (r *CPUGovernorResource) governorPaths() ([]string, error) {
	pattern := r.Env.Path("sys", "devices", "system", "cpu", "cpu[0-9]*", "cpufreq", "scaling_governor")
	return filepath.Glob(pattern)
}
