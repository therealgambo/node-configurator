// Package engine wires a resolved profile.Profile into concrete
// resource.Resource instances, runs them in order, and reports the outcome.
package engine

import (
	"context"

	"github.com/therealgambo/node-configurator/internal/facts"
	"github.com/therealgambo/node-configurator/internal/profile"
	"github.com/therealgambo/node-configurator/internal/resource"
	"github.com/therealgambo/node-configurator/internal/sysutil"
)

// Mode controls whether Run converges drift or only reports it.
type Mode int

const (
	ModeApply Mode = iota
	ModeCheck
)

// Options filters which resources Build includes.
type Options struct {
	Only []string // if non-empty, only these resource names run
	Skip []string // resource names to skip, unioned with the profile's own disabled list
}

const (
	sysctlPersistFile  = "etc/sysctl.d/99-node-configurator.conf"
	modulesPersistFile = "etc/modules-load.d/99-node-configurator.conf"
	limitsPersistFile  = "etc/security/limits.d/99-node-configurator.conf"
)

// systemdLimitServices lists the units whose LimitNOFILE this tool raises
// via a drop-in. Writing a drop-in for a unit that isn't installed yet is
// harmless — systemd just won't have anything to apply it to until the unit
// exists, which matters here since this tool can run before
// containerd/kubelet/Cilium are installed (e.g. from early boot user-data).
var systemdLimitServices = []string{"containerd", "kubelet", "cilium"}

// Build constructs the ordered list of resources for f/p, applying disabled
// (from operator config) and opts (from CLI flags) as include/exclude
// filters.
func Build(env *sysutil.Env, runner sysutil.Runner, f facts.Facts, p profile.Profile, disabled []string, opts Options) []resource.Resource {
	iface := env.PrimaryInterface()

	all := []resource.Resource{
		&resource.SysctlResource{
			Env:         env,
			Settings:    p.Sysctls,
			PersistFile: env.Path(sysctlPersistFile),
		},
		&resource.KernelModuleResource{
			Env:         env,
			Modules:     p.Modules,
			Runner:      runner,
			PersistFile: env.Path(modulesPersistFile),
		},
		&resource.SwapResource{Env: env, Runner: runner},
		&resource.CgroupV2Resource{Env: env},
	}

	for _, m := range p.Mounts {
		all = append(all, &resource.MountResource{
			Env: env, Runner: runner, ResourceName: m.Name,
			Device: m.Device, Path: m.Path, FSType: m.FSType,
			Options: m.Options, FstabPersist: m.FstabPersist,
		})
	}

	if p.Limits.NoFile > 0 {
		all = append(all, &resource.DropInFileResource{
			ResourceName: "limits-conf",
			Path:         env.Path(limitsPersistFile),
			Content:      resource.RenderLimitsConf(p.Limits.NoFile, p.Limits.NProc),
		})
		for _, svc := range systemdLimitServices {
			all = append(all, &resource.DropInFileResource{
				ResourceName: "systemd-limits-" + svc,
				Path:         env.Path("etc", "systemd", "system", svc+".service.d", "99-node-configurator.conf"),
				Content:      resource.RenderSystemdLimitDropIn(p.Limits.NoFile),
			})
		}
	}

	if p.IRQ.Enabled {
		all = append(all, &resource.IRQAffinityResource{Env: env, Interface: iface, NumCPUs: f.VCPUs})
	}

	if p.NIC.RingBufferMax || p.NIC.RPS || p.NIC.XPS {
		all = append(all, &resource.NICResource{
			Env: env, Runner: runner, Interface: iface, NumCPUs: f.VCPUs,
			RingBufferMax: p.NIC.RingBufferMax, RPS: p.NIC.RPS, XPS: p.NIC.XPS,
		})
	}

	if p.CPUGovernor != "" {
		all = append(all, &resource.CPUGovernorResource{Env: env, Governor: p.CPUGovernor})
	}

	if p.IOScheduler.Enabled {
		all = append(all, &resource.IOSchedulerResource{Env: env, Scheduler: p.IOScheduler.Scheduler})
	}

	return filterResources(all, disabled, opts)
}

func filterResources(all []resource.Resource, disabledFromConfig []string, opts Options) []resource.Resource {
	excluded := make(map[string]bool, len(disabledFromConfig)+len(opts.Skip))
	for _, name := range disabledFromConfig {
		excluded[name] = true
	}
	for _, name := range opts.Skip {
		excluded[name] = true
	}

	var included map[string]bool
	if len(opts.Only) > 0 {
		included = make(map[string]bool, len(opts.Only))
		for _, name := range opts.Only {
			included[name] = true
		}
	}

	filtered := make([]resource.Resource, 0, len(all))
	for _, r := range all {
		if excluded[r.Name()] {
			continue
		}
		if included != nil && !included[r.Name()] {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}

// Run executes Check (and, in ModeApply, Apply when Check found drift) for
// every resource in order. A failure in one resource does not stop the
// rest — an operator reading the report needs the full picture, and a
// systemd oneshot run should still converge everything it safely can.
func Run(ctx context.Context, resources []resource.Resource, mode Mode) []resource.Result {
	results := make([]resource.Result, 0, len(resources))
	for _, r := range resources {
		res, err := r.Check(ctx)
		if err != nil {
			results = append(results, res)
			continue
		}

		if mode == ModeApply && res.Status == resource.StatusWouldChange {
			if applyErr := r.Apply(ctx); applyErr != nil {
				res.Status = resource.StatusFailed
				res.Detail = applyErr.Error()
			} else {
				res.Status = resource.StatusChanged
			}
		}

		results = append(results, res)
	}
	return results
}
