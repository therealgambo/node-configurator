package profile

import "github.com/therealgambo/node-configurator/internal/facts"

// newBaseProfile returns the profile skeleton applied to every node,
// independent of instance size. Facts-dependent values are filled in by
// applyScaling and applyCiliumPrereqs.
func newBaseProfile(f facts.Facts) Profile {
	return Profile{
		Sysctls:     baseSysctls(),
		Modules:     []string{},
		Limits:      LimitsSpec{NoFile: 1048576, NProc: 0},
		Mounts:      []MountSpec{},
		HugePages:   HugePagesSpec{Enabled: false},
		IRQ:         IRQSpec{Enabled: f.VCPUs > 1},
		NIC:         NICSpec{RingBufferMax: true, RPS: true, XPS: true},
		CPUGovernor: "performance",
		IOScheduler: IOSchedulerSpec{Enabled: true, Scheduler: "none"},
		Swap:        SwapSpec{Disable: true},
	}
}

// baseSysctls are values that don't need to scale with instance size: pod
// churn and container/thread density on a Kubernetes node justify generous
// fixed ceilings regardless of how big the box is.
func baseSysctls() map[string]string {
	return map[string]string{
		// Memory-mapped regions: containerized JVMs, Elasticsearch-adjacent
		// workloads, and any process with many shared libraries or mmap'd
		// files can exceed the historical default of 65530.
		"vm.max_map_count": "262144",

		// Swap is force-disabled by the swap resource; this is defense in
		// depth in case swap is ever re-enabled out of band.
		"vm.swappiness": "0",

		// The Cilium agent and kubelet both watch large numbers of files
		// (per-pod cgroup/netns paths, config, secrets/configmap volumes).
		"fs.inotify.max_user_watches":   "1048576",
		"fs.inotify.max_user_instances": "8192",

		// Ceiling on open files any single process may request; the actual
		// soft/hard limits are set via the limits resource.
		"fs.nr_open": "1048576",

		// CNI plugins (including Cilium) require the host to forward
		// traffic between pod and external interfaces.
		"net.ipv4.ip_forward": "1",

		// eBPF eats into direct-mapped memory reported by the JIT; Cilium's
		// datapath is eBPF-heavy so keep JIT compilation on and unhardened
		// for maximum throughput (this host does not run untrusted code).
		"net.core.bpf_jit_enable": "1",
		"net.core.bpf_jit_harden": "0",

		// Defense in depth: only privileged (CAP_BPF) processes may load
		// BPF programs. Cilium runs privileged, so this does not impair it.
		"kernel.unprivileged_bpf_disabled": "1",

		// Prefer letting an ASG/health-check replace a node over it limping
		// along in a half-broken kernel state.
		"kernel.panic":         "10",
		"kernel.panic_on_oops": "1",
	}
}
