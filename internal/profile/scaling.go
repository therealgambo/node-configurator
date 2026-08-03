package profile

import (
	"fmt"

	"github.com/therealgambo/node-configurator/internal/facts"
)

// applyScaling adds sysctl values that scale with instance size (vCPUs,
// memory, network bandwidth tier, neighbor-table capacity) on top of the
// fixed baseline. Formulas below are conservative, documented starting
// points, not a tuned-for-every-workload bible — expect to refine the
// clamps after observing real cluster metrics.
func applyScaling(p *Profile, f facts.Facts) {
	memMiB := f.MemMiB
	if memMiB <= 0 {
		memMiB = 1024 // smallest realistic EC2 size; avoids a zero-value formula collapse
	}
	vcpus := f.VCPUs
	if vcpus <= 0 {
		vcpus = 1
	}
	bandwidthGbps := f.NetworkBandwidthGbps

	memGiB := float64(memMiB) / 1024.0

	fileMax := clampInt(int64(memGiB*131072), 1_048_576, 12_582_912)
	p.Sysctls["fs.file-max"] = fmt.Sprintf("%d", fileMax)

	somaxconn := clampInt(int64(vcpus)*2048, 4096, 65535)
	p.Sysctls["net.core.somaxconn"] = fmt.Sprintf("%d", somaxconn)

	threadsMax := clampInt(int64(vcpus)*4096, 131072, 4_194_304)
	p.Sysctls["kernel.threads-max"] = fmt.Sprintf("%d", threadsMax)
	p.Sysctls["kernel.pid_max"] = fmt.Sprintf("%d", clampInt(int64(vcpus)*4096, 131072, 4_194_304))

	backlog := clampInt(int64(bandwidthGbps*2000), 4096, 250_000)
	p.Sysctls["net.core.netdev_max_backlog"] = fmt.Sprintf("%d", backlog)

	rmemMax := clampInt(int64(bandwidthGbps)*2*1_048_576, 4_194_304, 134_217_728)
	wmemMax := rmemMax
	p.Sysctls["net.core.rmem_max"] = fmt.Sprintf("%d", rmemMax)
	p.Sysctls["net.core.wmem_max"] = fmt.Sprintf("%d", wmemMax)
	p.Sysctls["net.ipv4.tcp_rmem"] = fmt.Sprintf("4096 131072 %d", rmemMax)
	p.Sysctls["net.ipv4.tcp_wmem"] = fmt.Sprintf("4096 65536 %d", wmemMax)

	// eBPF datapath programs/maps scale with the number of endpoints the
	// Cilium agent manages, which is roughly proportional to pods-per-node,
	// which is itself bounded by vCPUs in practice.
	bpfJitLimit := clampInt(int64(vcpus)*8_388_608, 268_435_456, 1_073_741_824)
	p.Sysctls["net.core.bpf_jit_limit"] = fmt.Sprintf("%d", bpfJitLimit)

	// Neighbor table: sized off the maximum possible ENI/IP fan-out for this
	// instance type (a proxy for how many pods, and therefore distinct
	// peers, it can host) so pod churn doesn't silently drop packets once
	// the table fills. Real AWS ENI*IPs-per-ENI products range from ~4
	// (t3.micro) to ~1000 (24xlarge-class instances), so the multiplier is
	// large relative to the kernel's own default floors (128/512/1024) to
	// keep the formula meaningful across that whole range instead of every
	// instance size collapsing onto the floor.
	thresh3 := clampInt(int64(f.MaxNeighborEntries())*16, 1024, 131_072)
	thresh2 := max(thresh3/2, 512)
	thresh1 := max(thresh3/8, 128)
	p.Sysctls["net.ipv4.neigh.default.gc_thresh1"] = fmt.Sprintf("%d", thresh1)
	p.Sysctls["net.ipv4.neigh.default.gc_thresh2"] = fmt.Sprintf("%d", thresh2)
	p.Sysctls["net.ipv4.neigh.default.gc_thresh3"] = fmt.Sprintf("%d", thresh3)
}

func clampInt(v, min, max int64) int64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
