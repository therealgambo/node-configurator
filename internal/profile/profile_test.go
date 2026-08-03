package profile

import (
	"slices"
	"strconv"
	"testing"

	"github.com/therealgambo/node-configurator/internal/facts"
)

func sysctlInt(t *testing.T, p Profile, key string) int64 {
	t.Helper()
	raw, ok := p.Sysctls[key]
	if !ok {
		t.Fatalf("missing sysctl %s", key)
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		t.Fatalf("sysctl %s = %q is not an integer: %v", key, raw, err)
	}
	return v
}

func TestBuildScalesAcrossInstanceSizes(t *testing.T) {
	tests := []struct {
		name                 string
		facts                facts.Facts
		wantSomaxconnAtLeast int64
		wantSomaxconnAtMost  int64
		wantFileMaxAtLeast   int64
	}{
		{
			name:                 "t3.micro smallest burstable",
			facts:                facts.Facts{VCPUs: 2, MemMiB: 1024, NetworkBandwidthGbps: 0},
			wantSomaxconnAtLeast: 4096,
			wantSomaxconnAtMost:  4096,
			wantFileMaxAtLeast:   1_048_576,
		},
		{
			name:                 "m6i.xlarge general purpose",
			facts:                facts.Facts{VCPUs: 4, MemMiB: 16384, NetworkBandwidthGbps: 6.25},
			wantSomaxconnAtLeast: 8192,
			wantSomaxconnAtMost:  8192,
			wantFileMaxAtLeast:   1_048_576,
		},
		{
			name:                 "r5dn.24xlarge huge memory+network",
			facts:                facts.Facts{VCPUs: 96, MemMiB: 786432, NetworkBandwidthGbps: 75, MaxENIs: 15, Ipv4AddressesPerENI: 50},
			wantSomaxconnAtLeast: 65535, // clamped to max
			wantSomaxconnAtMost:  65535,
			wantFileMaxAtLeast:   12_582_912, // clamped to max
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := Build(tc.facts, Overrides{})

			got := sysctlInt(t, p, "net.core.somaxconn")
			if got < tc.wantSomaxconnAtLeast || got > tc.wantSomaxconnAtMost {
				t.Errorf("net.core.somaxconn = %d, want between %d and %d", got, tc.wantSomaxconnAtLeast, tc.wantSomaxconnAtMost)
			}

			fileMax := sysctlInt(t, p, "fs.file-max")
			if fileMax < tc.wantFileMaxAtLeast {
				t.Errorf("fs.file-max = %d, want at least %d", fileMax, tc.wantFileMaxAtLeast)
			}
			if fileMax > 12_582_912 {
				t.Errorf("fs.file-max = %d, exceeds documented clamp of 12582912", fileMax)
			}
		})
	}
}

func TestBuildNeighborTableScalesWithENIFanout(t *testing.T) {
	small := Build(facts.Facts{VCPUs: 2, MemMiB: 1024, MaxENIs: 2, Ipv4AddressesPerENI: 2}, Overrides{})
	large := Build(facts.Facts{VCPUs: 96, MemMiB: 786432, MaxENIs: 15, Ipv4AddressesPerENI: 50}, Overrides{})

	smallThresh3 := sysctlInt(t, small, "net.ipv4.neigh.default.gc_thresh3")
	largeThresh3 := sysctlInt(t, large, "net.ipv4.neigh.default.gc_thresh3")
	if largeThresh3 <= smallThresh3 {
		t.Fatalf("expected larger instance to have a bigger neighbor table: small=%d large=%d", smallThresh3, largeThresh3)
	}

	// thresh1 < thresh2 < thresh3 must always hold, or the kernel's neighbor
	// garbage collector behaves unpredictably.
	t1 := sysctlInt(t, large, "net.ipv4.neigh.default.gc_thresh1")
	t2 := sysctlInt(t, large, "net.ipv4.neigh.default.gc_thresh2")
	t3 := sysctlInt(t, large, "net.ipv4.neigh.default.gc_thresh3")
	if t1 >= t2 || t2 >= t3 {
		t.Fatalf("expected gc_thresh1 < gc_thresh2 < gc_thresh3, got %d %d %d", t1, t2, t3)
	}
}

func TestBuildAppliesBaselineRegardlessOfSize(t *testing.T) {
	p := Build(facts.Facts{VCPUs: 1, MemMiB: 512}, Overrides{})
	for _, key := range []string{"vm.max_map_count", "vm.swappiness", "net.ipv4.ip_forward", "kernel.unprivileged_bpf_disabled"} {
		if _, ok := p.Sysctls[key]; !ok {
			t.Errorf("expected baseline sysctl %s to be present", key)
		}
	}
	if !p.Swap.Disable {
		t.Error("expected swap disabled by default")
	}
	if p.CPUGovernor != "performance" {
		t.Errorf("expected default CPU governor performance, got %q", p.CPUGovernor)
	}
}

func TestBuildOmitsKubeProxyModulesByDefault(t *testing.T) {
	p := Build(facts.Facts{VCPUs: 4, MemMiB: 16384}, Overrides{})
	for _, mod := range []string{"nf_conntrack", "br_netfilter", "ip_vs"} {
		for _, m := range p.Modules {
			if m == mod {
				t.Errorf("expected no kube-proxy-oriented module %s to be loaded by default (Cilium eBPF fleet)", mod)
			}
		}
	}
	if _, ok := p.Sysctls["net.bridge.bridge-nf-call-iptables"]; ok {
		t.Error("expected no bridge-nf sysctl by default")
	}
}

func TestBuildCiliumTunnelModeGatesModule(t *testing.T) {
	native := Build(facts.Facts{VCPUs: 4, MemMiB: 16384}, Overrides{Cilium: CiliumOptions{TunnelMode: ""}})
	if len(native.Modules) != 0 {
		t.Errorf("expected no tunnel module for native routing, got %v", native.Modules)
	}

	vxlan := Build(facts.Facts{VCPUs: 4, MemMiB: 16384}, Overrides{Cilium: CiliumOptions{TunnelMode: "vxlan"}})
	if !containsString(vxlan.Modules, "vxlan") {
		t.Errorf("expected vxlan module when TunnelMode=vxlan, got %v", vxlan.Modules)
	}

	legacy := Build(facts.Facts{VCPUs: 4, MemMiB: 16384}, Overrides{Cilium: CiliumOptions{LegacyIPTables: true}})
	if !containsString(legacy.Modules, "br_netfilter") {
		t.Errorf("expected br_netfilter when LegacyIPTables=true, got %v", legacy.Modules)
	}
	if legacy.Sysctls["net.bridge.bridge-nf-call-iptables"] != "1" {
		t.Error("expected bridge-nf-call-iptables=1 when LegacyIPTables=true")
	}
}

func TestBuildAlwaysMountsBpffs(t *testing.T) {
	p := Build(facts.Facts{VCPUs: 4, MemMiB: 16384}, Overrides{})
	found := false
	for _, m := range p.Mounts {
		if m.Path == "/sys/fs/bpf" && m.FSType == "bpffs" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected bpffs mount at /sys/fs/bpf, got %+v", p.Mounts)
	}
}

func TestOverridesTakePrecedence(t *testing.T) {
	governor := ""
	p := Build(facts.Facts{VCPUs: 4, MemMiB: 16384}, Overrides{
		SysctlOverrides: map[string]string{"net.core.somaxconn": "1234", "net.ipv4.tcp_congestion_control": "bbr"},
		NoFileLimit:     2_097_152,
		CPUGovernor:     &governor,
		HugePages2M:     512,
	})

	if p.Sysctls["net.core.somaxconn"] != "1234" {
		t.Errorf("expected override to win over computed default, got %s", p.Sysctls["net.core.somaxconn"])
	}
	if p.Sysctls["net.ipv4.tcp_congestion_control"] != "bbr" {
		t.Errorf("expected override-only key to be present, got %v", p.Sysctls)
	}
	if p.Limits.NoFile != 2_097_152 {
		t.Errorf("expected NoFile override, got %d", p.Limits.NoFile)
	}
	if p.CPUGovernor != "" {
		t.Errorf("expected CPU governor disabled by override, got %q", p.CPUGovernor)
	}
	if !p.HugePages.Enabled || p.HugePages.Count2M != 512 {
		t.Errorf("expected hugepages enabled with 512 pages, got %+v", p.HugePages)
	}
	if p.Sysctls["vm.nr_hugepages"] != "512" {
		t.Errorf("expected vm.nr_hugepages sysctl to carry the hugepages count, got %q", p.Sysctls["vm.nr_hugepages"])
	}
}

func TestOverridesFeatureTogglesTakePrecedence(t *testing.T) {
	disabled := false
	ioSched := ""
	p := Build(facts.Facts{VCPUs: 8, MemMiB: 16384}, Overrides{
		IRQAffinity:   &disabled,
		RingBufferMax: &disabled,
		RPS:           &disabled,
		XPS:           &disabled,
		IOScheduler:   &ioSched,
	})

	if p.IRQ.Enabled {
		t.Error("expected IRQ affinity disabled by override despite vcpus>1")
	}
	if p.NIC.RingBufferMax || p.NIC.RPS || p.NIC.XPS {
		t.Errorf("expected all NIC feature toggles disabled by override, got %+v", p.NIC)
	}
	if p.IOScheduler.Enabled {
		t.Error("expected IO scheduler resource disabled when override sets an empty scheduler")
	}
}

func containsString(haystack []string, needle string) bool {
	return slices.Contains(haystack, needle)
}
