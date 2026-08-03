package resource

import (
	"context"
	"testing"
)

func TestCgroupV2ResourceUnifiedIsUnchanged(t *testing.T) {
	env := newTestEnv(t)
	writeFile(t, env.Path("sys", "fs", "cgroup", "cgroup.controllers"), "cpu memory\n")

	r := &CgroupV2Resource{Env: env}
	res, err := r.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusUnchanged {
		t.Fatalf("expected unchanged, got %s", res.Status)
	}
}

func TestCgroupV2ResourceHybridReportsRebootRequired(t *testing.T) {
	env := newTestEnv(t)

	r := &CgroupV2Resource{Env: env}
	res, err := r.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusRebootRequired {
		t.Fatalf("expected reboot_required, got %s", res.Status)
	}

	// Apply must be a safe no-op — this tool never edits the bootloader or reboots.
	if err := r.Apply(context.Background()); err != nil {
		t.Fatalf("expected Apply to be a no-op, got error: %v", err)
	}
}
