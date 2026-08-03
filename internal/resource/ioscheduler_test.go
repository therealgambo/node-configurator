package resource

import (
	"context"
	"os"
	"testing"
)

func TestIOSchedulerResourceSkippedWhenDisabled(t *testing.T) {
	env := newTestEnv(t)
	r := &IOSchedulerResource{Env: env, Scheduler: ""}
	res, err := r.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusSkipped {
		t.Fatalf("expected skipped, got %s", res.Status)
	}
}

func TestIOSchedulerResourceSkippedWithNoNVMeDevices(t *testing.T) {
	env := newTestEnv(t)
	r := &IOSchedulerResource{Env: env, Scheduler: "none"}
	res, err := r.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusSkipped {
		t.Fatalf("expected skipped when no NVMe devices, got %s", res.Status)
	}
}

func TestIOSchedulerResourceIgnoresPartitions(t *testing.T) {
	env := newTestEnv(t)
	schedPath := env.Path("sys", "block", "nvme0n1", "queue", "scheduler")
	writeFile(t, schedPath, "[mq-deadline] kyber none\n")
	// A partition entry should never be treated as its own device.
	partPath := env.Path("sys", "block", "nvme0n1p1", "queue", "scheduler")
	writeFile(t, partPath, "[mq-deadline] kyber none\n")

	r := &IOSchedulerResource{Env: env, Scheduler: "none"}
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

	data, err := os.ReadFile(schedPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "none" {
		t.Fatalf("expected nvme0n1 scheduler set to none, got %q", data)
	}

	// Partition's own "scheduler" file must be untouched (queue is shared with
	// the parent device in reality, but this test only verifies our code never
	// targets nvme0n1p1 as if it were an independent whole-disk device).
	partData, err := os.ReadFile(partPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(partData) != "[mq-deadline] kyber none\n" {
		t.Fatalf("expected partition scheduler file untouched, got %q", partData)
	}
}

func TestIOSchedulerResourceSkipsUnavailableScheduler(t *testing.T) {
	env := newTestEnv(t)
	schedPath := env.Path("sys", "block", "nvme0n1", "queue", "scheduler")
	writeFile(t, schedPath, "[mq-deadline] kyber\n") // "none" not offered

	r := &IOSchedulerResource{Env: env, Scheduler: "none"}
	res, err := r.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusUnchanged {
		t.Fatalf("expected unchanged when scheduler unavailable, got %s (%s)", res.Status, res.Detail)
	}
}
