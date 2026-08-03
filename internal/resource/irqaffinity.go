package resource

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/therealgambo/node-configurator/internal/sysutil"
)

// IRQAffinityResource spreads ENA NIC queue interrupts round-robin across
// online vCPUs. On smaller instance sizes the number of hardware queues can
// be less than the vCPU count (and vice versa on larger ones); either way,
// leaving every queue's IRQ on its kernel-assigned default (often all
// funneled onto CPU0) leaves throughput on the table under load.
//
// This does not attempt to respect isolcpus/NOHZ_FULL CPU isolation — that
// is out of scope for a general Kubernetes worker node profile.
type IRQAffinityResource struct {
	Env       *sysutil.Env
	Interface string
	NumCPUs   int
}

func (r *IRQAffinityResource) Name() string { return "irq-affinity" }

func (r *IRQAffinityResource) Check(ctx context.Context) (Result, error) {
	if r.Interface == "" || r.NumCPUs <= 1 {
		return Result{Name: r.Name(), Status: StatusSkipped, Detail: "no interface or only one vCPU, nothing to spread"}, nil
	}

	irqs, err := r.enaIRQs()
	if err != nil {
		return Result{Name: r.Name(), Status: StatusFailed, Detail: err.Error()}, err
	}
	if len(irqs) == 0 {
		return Result{Name: r.Name(), Status: StatusSkipped, Detail: fmt.Sprintf("no queue IRQs found for %s", r.Interface)}, nil
	}

	want := r.desiredAffinity(irqs)
	var drift []string
	for _, irq := range irqs {
		cur, err := sysutil.ReadFileString(r.affinityPath(irq))
		if err != nil {
			continue // e.g. IRQ vector disappeared between listing and reading; skip rather than fail the whole resource
		}
		cur = strings.TrimSpace(cur)
		wantStr := strconv.Itoa(want[irq])
		if cur != wantStr {
			drift = append(drift, fmt.Sprintf("irq %s: %s -> %s", irq, cur, wantStr))
		}
	}

	if len(drift) == 0 {
		return Result{Name: r.Name(), Status: StatusUnchanged}, nil
	}
	return Result{Name: r.Name(), Status: StatusWouldChange, Detail: strings.Join(drift, ", ")}, nil
}

func (r *IRQAffinityResource) Apply(ctx context.Context) error {
	irqs, err := r.enaIRQs()
	if err != nil {
		return err
	}
	want := r.desiredAffinity(irqs)
	for _, irq := range irqs {
		path := r.affinityPath(irq)
		cur, err := sysutil.ReadFileString(path)
		wantStr := strconv.Itoa(want[irq])
		if err == nil && strings.TrimSpace(cur) == wantStr {
			continue
		}
		if err := os.WriteFile(path, []byte(wantStr), 0o644); err != nil {
			return fmt.Errorf("setting affinity for irq %s: %w", irq, err)
		}
	}
	return nil
}

func (r *IRQAffinityResource) affinityPath(irq string) string {
	return r.Env.Path("proc", "irq", irq, "smp_affinity_list")
}

// desiredAffinity assigns each IRQ a single CPU, round-robin, so queues are
// deterministically spread rather than mask-shared across every CPU (which
// leaves the kernel's own IRQ balancer to decide, defeating the point).
func (r *IRQAffinityResource) desiredAffinity(irqs []string) map[string]int {
	n := r.NumCPUs
	if n <= 0 {
		n = 1
	}
	want := make(map[string]int, len(irqs))
	for i, irq := range irqs {
		want[irq] = i % n
	}
	return want
}

// enaIRQs parses /proc/interrupts for queue IRQs belonging to r.Interface,
// identified by the ENA driver's "<iface>-Tx-Rx-<n>" (or "<iface>-Rx-<n>"/
// "<iface>-Tx-<n>" on older driver versions) queue-name convention.
func (r *IRQAffinityResource) enaIRQs() ([]string, error) {
	f, err := os.Open(r.Env.Path("proc", "interrupts"))
	if err != nil {
		return nil, fmt.Errorf("reading /proc/interrupts: %w", err)
	}
	defer func() { _ = f.Close() }()

	prefix := r.Interface + "-"
	var irqs []int
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			continue
		}
		desc := fields[len(fields)-1]
		if !strings.HasPrefix(desc, prefix) {
			continue
		}
		irqNum, err := strconv.Atoi(strings.TrimSuffix(fields[0], ":"))
		if err != nil {
			continue // header row or non-numeric IRQ (e.g. "NMI:"), not a queue IRQ
		}
		irqs = append(irqs, irqNum)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	sort.Ints(irqs)
	result := make([]string, len(irqs))
	for i, irq := range irqs {
		result[i] = strconv.Itoa(irq)
	}
	return result, nil
}
