package resource

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestIRQAffinityResourceSkipsSingleCPU(t *testing.T) {
	env := newTestEnv(t)
	r := &IRQAffinityResource{Env: env, Interface: "eth0", NumCPUs: 1}
	res, err := r.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusSkipped {
		t.Fatalf("expected skipped for single CPU, got %s", res.Status)
	}
}

func TestIRQAffinityResourceSpreadsAcrossCPUs(t *testing.T) {
	env := newTestEnv(t)
	interrupts := ` CPU0  CPU1  CPU2  CPU3
 45:   1000     0     0     0   PCI-MSI 512000-edge   eth0-Tx-Rx-0
 46:      0  1000     0     0   PCI-MSI 512001-edge   eth0-Tx-Rx-1
 47:      0     0  1000     0   PCI-MSI 512002-edge   eth0-Tx-Rx-2
 NMI:      0     0     0     0   Non-maskable interrupts
`
	writeFile(t, env.Path("proc", "interrupts"), interrupts)
	writeFile(t, env.Path("proc", "irq", "45", "smp_affinity_list"), "0-3")
	writeFile(t, env.Path("proc", "irq", "46", "smp_affinity_list"), "0-3")
	writeFile(t, env.Path("proc", "irq", "47", "smp_affinity_list"), "0-3")

	r := &IRQAffinityResource{Env: env, Interface: "eth0", NumCPUs: 4}

	res, err := r.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusWouldChange {
		t.Fatalf("expected would_change, got %s (%s)", res.Status, res.Detail)
	}

	if err := r.Apply(context.Background()); err != nil {
		t.Fatalf("apply: %v", err)
	}

	want := map[string]string{"45": "0", "46": "1", "47": "2"}
	for irq, wantCPU := range want {
		data, err := os.ReadFile(env.Path("proc", "irq", irq, "smp_affinity_list"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(data)) != wantCPU {
			t.Errorf("irq %s: got %q, want %q", irq, data, wantCPU)
		}
	}

	res, err = r.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusUnchanged {
		t.Fatalf("expected unchanged after apply, got %s (%s)", res.Status, res.Detail)
	}
}

func TestIRQAffinityResourceNoMatchingIRQsSkipped(t *testing.T) {
	env := newTestEnv(t)
	writeFile(t, env.Path("proc", "interrupts"), " CPU0\n 1: 0 IO-APIC timer\n")

	r := &IRQAffinityResource{Env: env, Interface: "eth0", NumCPUs: 4}
	res, err := r.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusSkipped {
		t.Fatalf("expected skipped, got %s", res.Status)
	}
}
