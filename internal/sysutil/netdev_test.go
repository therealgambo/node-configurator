package sysutil

import (
	"os"
	"testing"
)

// TestPrimaryInterfacePrefersRealDeviceOverVirtual reproduces a real bug
// found running against a live CI runner: os.ReadDir returns entries in
// alphabetical order, and "docker0" (a software bridge, no backing
// hardware) sorts before "eth0" (the real NIC), so a naive "first
// non-loopback interface" pick chose the wrong one. A Kubernetes node
// running Cilium has the same problem via cilium_host/cilium_net/lxc+.
func TestPrimaryInterfacePrefersRealDeviceOverVirtual(t *testing.T) {
	env := &Env{Root: t.TempDir()}

	for _, iface := range []string{"lo", "docker0", "eth0"} {
		if err := os.MkdirAll(env.Path("sys", "class", "net", iface), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Only eth0 has a backing hardware device, matching a real ENA NIC
	// (or any physical/paravirtualized NIC) versus a software bridge.
	if err := os.MkdirAll(env.Path("sys", "class", "net", "eth0", "device"), 0o755); err != nil {
		t.Fatal(err)
	}

	if got := env.PrimaryInterface(); got != "eth0" {
		t.Fatalf("expected eth0 (has a device symlink), got %q", got)
	}
}

func TestPrimaryInterfaceFallsBackWithoutAnyRealDevice(t *testing.T) {
	env := &Env{Root: t.TempDir()}

	for _, iface := range []string{"lo", "docker0", "veth123"} {
		if err := os.MkdirAll(env.Path("sys", "class", "net", iface), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// None of these have a device symlink; fall back to the first
	// non-loopback entry rather than returning nothing.
	if got := env.PrimaryInterface(); got != "docker0" {
		t.Fatalf("expected fallback to first non-loopback entry docker0, got %q", got)
	}
}

func TestPrimaryInterfaceEmptyWhenNoInterfaces(t *testing.T) {
	env := &Env{Root: t.TempDir()}
	if err := os.MkdirAll(env.Path("sys", "class", "net"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := env.PrimaryInterface(); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}
