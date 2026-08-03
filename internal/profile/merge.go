package profile

import (
	"fmt"
	"maps"

	"github.com/therealgambo/node-configurator/internal/facts"
)

// Overrides carries operator-supplied adjustments layered on top of the
// computed profile. They always take precedence over facts-derived
// defaults. internal/config populates this from the operator's YAML file so
// this package stays config-format-agnostic and independently testable.
type Overrides struct {
	SysctlOverrides map[string]string
	NoFileLimit     uint64  // 0 = keep the computed default
	CPUGovernor     *string // nil = keep computed default; non-nil (including "") replaces it, "" disables the resource
	HugePages2M     int     // > 0 enables hugepages with this many 2M pages
	Cilium          CiliumOptions

	// Per-feature toggles within the NIC/IRQ/IOScheduler resources. nil
	// means "keep the computed default"; a non-nil value always wins, even
	// if that means disabling a feature the computed profile would have
	// enabled.
	IRQAffinity   *bool
	RingBufferMax *bool
	RPS           *bool
	XPS           *bool
	IOScheduler   *string // nil = keep computed default ("none"); non-nil (including "") replaces it, "" disables the resource
}

// Build computes the fully-resolved desired-state Profile for f, with ov
// applied last (highest precedence). Merge order: base -> size/bandwidth
// scaling -> Cilium prerequisites -> operator overrides.
func Build(f facts.Facts, ov Overrides) Profile {
	p := newBaseProfile(f)
	applyScaling(&p, f)
	applyCiliumPrereqs(&p, ov.Cilium)
	applyOverrides(&p, ov)
	return p
}

func applyOverrides(p *Profile, ov Overrides) {
	maps.Copy(p.Sysctls, ov.SysctlOverrides)
	if ov.NoFileLimit > 0 {
		p.Limits.NoFile = ov.NoFileLimit
	}
	if ov.CPUGovernor != nil {
		p.CPUGovernor = *ov.CPUGovernor
	}
	if ov.HugePages2M > 0 {
		p.HugePages = HugePagesSpec{Enabled: true, Count2M: ov.HugePages2M}
		// vm.nr_hugepages is a plain sysctl, so enforcement rides on the
		// same SysctlResource as everything else rather than needing a
		// bespoke resource.
		p.Sysctls["vm.nr_hugepages"] = fmt.Sprintf("%d", ov.HugePages2M)
	}
	if ov.IRQAffinity != nil {
		p.IRQ.Enabled = *ov.IRQAffinity
	}
	if ov.RingBufferMax != nil {
		p.NIC.RingBufferMax = *ov.RingBufferMax
	}
	if ov.RPS != nil {
		p.NIC.RPS = *ov.RPS
	}
	if ov.XPS != nil {
		p.NIC.XPS = *ov.XPS
	}
	if ov.IOScheduler != nil {
		p.IOScheduler.Scheduler = *ov.IOScheduler
		p.IOScheduler.Enabled = *ov.IOScheduler != ""
	}
}
