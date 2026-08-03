package sysutil

import "os"

// PrimaryInterface returns the network interface to tune: the first
// non-loopback entry under /sys/class/net that is backed by a real
// hardware device (i.e. has a "device" symlink), preferring that over
// software-only interfaces.
//
// This distinction matters a lot in practice: os.ReadDir returns entries in
// alphabetical order, and a Kubernetes worker node running Cilium always
// has software interfaces that sort before the real NIC -- "docker0" (if
// Docker is also installed), "cilium_host"/"cilium_net", "lxc+" veth pairs,
// etc. None of those have a backing PCI/VMBus device the way an ENA NIC
// (or any real physical/paravirtualized NIC) does, so picking "just the
// first non-loopback entry" would tune the wrong interface -- as happened
// during development, where a CI runner's "docker0" was picked over its
// real "eth0".
func (e *Env) PrimaryInterface() string {
	entries, err := os.ReadDir(e.Path("sys", "class", "net"))
	if err != nil {
		return ""
	}

	fallback := ""
	for _, entry := range entries {
		name := entry.Name()
		if name == "lo" {
			continue
		}
		if fallback == "" {
			fallback = name
		}
		if _, err := os.Stat(e.Path("sys", "class", "net", name, "device")); err == nil {
			return name
		}
	}
	return fallback
}
