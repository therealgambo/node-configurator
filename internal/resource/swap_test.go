package resource

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSwapResourceNoActiveSwapUnchanged(t *testing.T) {
	env := newTestEnv(t)
	if err := os.MkdirAll(env.Path("proc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env.Path("proc", "swaps"), []byte("Filename\tType\tSize\tUsed\tPriority\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(env.Path("etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env.Path("etc", "fstab"), []byte("/dev/root / ext4 rw 0 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := &SwapResource{Env: env, Runner: &fakeRunner{}}
	res, err := r.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusUnchanged {
		t.Fatalf("expected unchanged, got %s (%s)", res.Status, res.Detail)
	}
}

func TestSwapResourceDisablesActiveSwap(t *testing.T) {
	env := newTestEnv(t)
	if err := os.MkdirAll(env.Path("proc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env.Path("proc", "swaps"), []byte("Filename\tType\tSize\tUsed\tPriority\n/dev/nvme0n1p2\tpartition\t2097148\t0\t-2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(env.Path("etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	fstabPath := env.Path("etc", "fstab")
	if err := os.WriteFile(fstabPath, []byte("/dev/root / ext4 rw 0 0\n/dev/nvme0n1p2 none swap sw 0 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := &fakeRunner{}
	r := &SwapResource{Env: env, Runner: runner}

	res, err := r.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusWouldChange {
		t.Fatalf("expected would_change, got %s", res.Status)
	}

	if err := r.Apply(context.Background()); err != nil {
		t.Fatalf("apply: %v", err)
	}

	foundSwapoff := false
	for _, call := range runner.calls {
		if len(call) > 0 && call[0] == "swapoff" {
			foundSwapoff = true
		}
	}
	if !foundSwapoff {
		t.Fatalf("expected swapoff -a to be called, got %v", runner.calls)
	}

	fstab, err := os.ReadFile(fstabPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(fstab), "# disabled by node-configurator: /dev/nvme0n1p2 none swap sw 0 0") {
		t.Fatalf("expected swap fstab line to be commented out, got: %s", fstab)
	}
	if !strings.Contains(string(fstab), "/dev/root / ext4 rw 0 0") {
		t.Fatalf("expected non-swap fstab line to survive untouched, got: %s", fstab)
	}

	backup := fstabPath + ".node-configurator.orig"
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("expected fstab backup to be created: %v", err)
	}
}

func TestFstabHasActiveSwapLinesIgnoresComments(t *testing.T) {
	content := "# /dev/old swap swap sw 0 0\n/dev/root / ext4 rw 0 0\n"
	if fstabHasActiveSwapLines(content) {
		t.Fatal("expected commented-out swap line to not count as active")
	}
}

func TestSwapResourceMasksActiveUnits(t *testing.T) {
	env := newTestEnv(t)
	if err := os.MkdirAll(env.Path("proc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env.Path("proc", "swaps"), []byte("Filename\tType\tSize\tUsed\tPriority\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(env.Path("etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env.Path("etc", "fstab"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := &fakeRunner{listUnitsOutput: "dev-nvme0n1p2.swap loaded active active Swap\n"}
	r := &SwapResource{Env: env, Runner: runner}

	res, err := r.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusWouldChange {
		t.Fatalf("expected would_change due to active swap unit, got %s (%s)", res.Status, res.Detail)
	}

	if err := r.Apply(context.Background()); err != nil {
		t.Fatalf("apply: %v", err)
	}

	found := false
	for _, call := range runner.calls {
		if len(call) == 3 && call[0] == "systemctl" && call[1] == "mask" && call[2] == "dev-nvme0n1p2.swap" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected systemctl mask of active swap unit, got %v", runner.calls)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
