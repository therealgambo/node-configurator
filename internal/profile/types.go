// Package profile computes the fully-resolved desired-state Profile for a
// host from facts.Facts (instance family/size, distro, kernel) plus
// operator Overrides. It contains no I/O — it is pure data + formulas, kept
// fully unit-testable across representative instance types without a Linux
// host.
package profile

// Profile is the desired state the engine converges the host towards.
type Profile struct {
	Sysctls     map[string]string
	Modules     []string
	Limits      LimitsSpec
	Mounts      []MountSpec
	HugePages   HugePagesSpec
	IRQ         IRQSpec
	NIC         NICSpec
	CPUGovernor string // "" disables the resource entirely
	IOScheduler IOSchedulerSpec
	Swap        SwapSpec
}

// LimitsSpec configures process resource limits (ulimits).
type LimitsSpec struct {
	NoFile uint64 // 0 disables the limits resource
	NProc  uint64 // 0 means "unlimited" (rendered as the literal "infinity")
}

// MountSpec describes a filesystem that must be mounted (and optionally
// persisted to /etc/fstab) for the node to function, e.g. bpffs for Cilium.
type MountSpec struct {
	Name         string
	Device       string
	Path         string
	FSType       string
	Options      string
	FstabPersist bool
}

// HugePagesSpec configures runtime-settable 2M hugepage reservation.
// 1G hugepages require a kernel command-line change and are out of scope
// for a tool that never reboots the node.
type HugePagesSpec struct {
	Enabled bool
	Count2M int
}

// IRQSpec controls ENA interrupt-affinity spreading.
type IRQSpec struct {
	Enabled bool
}

// NICSpec controls ENA ring-buffer/RPS/XPS tuning.
type NICSpec struct {
	RingBufferMax bool
	RPS           bool
	XPS           bool
}

// IOSchedulerSpec controls the block-device I/O scheduler for NVMe devices.
type IOSchedulerSpec struct {
	Enabled   bool
	Scheduler string // e.g. "none"
}

// SwapSpec controls swap enforcement. Kubernetes nodes in this fleet run
// without swap (NodeSwap is a non-goal here — see plan notes).
type SwapSpec struct {
	Disable bool
}
