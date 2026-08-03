package resource

import (
	"context"
	"os"
	"testing"
)

func TestCPUGovernorResourceSkippedWhenAbsent(t *testing.T) {
	env := newTestEnv(t)
	r := &CPUGovernorResource{Env: env, Governor: "performance"}
	res, err := r.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusSkipped {
		t.Fatalf("expected skipped when no cpufreq sysfs present, got %s", res.Status)
	}
}

func TestCPUGovernorResourceDisabled(t *testing.T) {
	env := newTestEnv(t)
	r := &CPUGovernorResource{Env: env, Governor: ""}
	res, err := r.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusSkipped {
		t.Fatalf("expected skipped when disabled, got %s", res.Status)
	}
}

func TestCPUGovernorResourceAppliesToAllCPUs(t *testing.T) {
	env := newTestEnv(t)
	p0 := env.Path("sys", "devices", "system", "cpu", "cpu0", "cpufreq", "scaling_governor")
	p1 := env.Path("sys", "devices", "system", "cpu", "cpu1", "cpufreq", "scaling_governor")
	writeFile(t, p0, "powersave")
	writeFile(t, p1, "performance") // already correct

	r := &CPUGovernorResource{Env: env, Governor: "performance"}
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

	data, err := os.ReadFile(p0)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "performance" {
		t.Fatalf("expected cpu0 governor set to performance, got %q", data)
	}

	res, err = r.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusUnchanged {
		t.Fatalf("expected unchanged after apply, got %s", res.Status)
	}
}
