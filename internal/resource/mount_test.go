package resource

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/therealgambo/node-configurator/internal/sysutil"
)

func writeProcMounts(t *testing.T, env *sysutil.Env, lines ...string) {
	t.Helper()
	path := env.Path("proc", "mounts")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := strings.Join(lines, "\n")
	if content != "" {
		content += "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMountResourceAlreadyMounted(t *testing.T) {
	env := newTestEnv(t)
	writeProcMounts(t, env, "bpffs /sys/fs/bpf bpffs rw,nosuid,nodev,noexec,relatime 0 0")

	r := &MountResource{
		Env: env, Runner: &fakeRunner{}, ResourceName: "bpffs-mount",
		Device: "bpffs", Path: "/sys/fs/bpf", FSType: "bpffs",
	}
	res, err := r.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusUnchanged {
		t.Fatalf("expected unchanged, got %s (%s)", res.Status, res.Detail)
	}
}

func TestMountResourceNotMountedApplies(t *testing.T) {
	env := newTestEnv(t)
	writeProcMounts(t, env, "/dev/root / ext4 rw,relatime 0 0")

	if err := os.MkdirAll(env.Path("etc"), 0o755); err != nil {
		t.Fatal(err)
	}

	runner := &fakeRunner{}
	r := &MountResource{
		Env: env, Runner: runner, ResourceName: "bpffs-mount",
		Device: "bpffs", Path: filepath.Join(t.TempDir(), "bpf"), FSType: "bpffs",
		Options: "rw,nosuid,nodev,noexec,relatime", FstabPersist: true,
	}

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

	if len(runner.calls) != 1 || runner.calls[0][0] != "mount" {
		t.Fatalf("expected one mount call, got %v", runner.calls)
	}

	fstab, err := os.ReadFile(env.Path("etc", "fstab"))
	if err != nil {
		t.Fatalf("expected fstab to be written: %v", err)
	}
	if !strings.Contains(string(fstab), r.Path) {
		t.Fatalf("fstab missing mount point: %s", fstab)
	}

	// Re-running persistFstab must not duplicate the entry.
	if err := r.persistFstab(); err != nil {
		t.Fatal(err)
	}
	fstab2, err := os.ReadFile(env.Path("etc", "fstab"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(fstab2), r.Path) != 1 {
		t.Fatalf("expected exactly one fstab entry, got: %s", fstab2)
	}
}
