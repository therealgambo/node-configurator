package profile

// bpffsPath is where Cilium expects the BPF filesystem mounted so its maps
// survive agent restarts.
const bpffsPath = "/sys/fs/bpf"

// CiliumOptions controls the host prerequisites prepared for a
// kube-proxy-free, eBPF-mode Cilium install. Defaults assume native/ENI
// routing with the strict eBPF kube-proxy replacement: no netfilter-based
// conntrack/masquerade modules are loaded unless explicitly opted into,
// since this fleet does not run kube-proxy.
type CiliumOptions struct {
	TunnelMode     string // "" (native/ENI routing, default), "vxlan", or "geneve"
	LegacyIPTables bool   // true only if Cilium is configured with --enable-iptables-rules
}

// applyCiliumPrereqs adds the host-level mounts/modules/sysctls Cilium
// needs before its agent starts. It does not configure Cilium itself.
func applyCiliumPrereqs(p *Profile, opts CiliumOptions) {
	p.Mounts = append(p.Mounts, MountSpec{
		Name:         "bpffs-mount",
		Device:       "bpffs",
		Path:         bpffsPath,
		FSType:       "bpffs",
		Options:      "rw,nosuid,nodev,noexec,relatime",
		FstabPersist: true,
	})

	switch opts.TunnelMode {
	case "vxlan":
		p.Modules = append(p.Modules, "vxlan")
	case "geneve":
		p.Modules = append(p.Modules, "geneve")
	}

	// Only load netfilter-oriented modules/sysctls for installs that still
	// rely on iptables masquerading; the strict eBPF kube-proxy replacement
	// this fleet uses bypasses netfilter conntrack entirely.
	if opts.LegacyIPTables {
		p.Modules = append(p.Modules, "br_netfilter", "nf_conntrack", "iptable_nat", "iptable_filter")
		p.Sysctls["net.bridge.bridge-nf-call-iptables"] = "1"
		p.Sysctls["net.bridge.bridge-nf-call-ip6tables"] = "1"
	}
}
