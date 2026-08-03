package sysutil

import "os"

// PrimaryInterface returns the first non-loopback network interface found
// under /sys/class/net. Standard Kubernetes worker nodes on EC2 have a
// single ENA NIC, so "first non-loopback" is a reliable default for "the
// interface to tune" without needing route-table inspection.
func (e *Env) PrimaryInterface() string {
	entries, err := os.ReadDir(e.Path("sys", "class", "net"))
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.Name() != "lo" {
			return entry.Name()
		}
	}
	return ""
}
