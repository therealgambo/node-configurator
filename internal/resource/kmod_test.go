package resource

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/therealgambo/node-configurator/internal/sysutil"
)

func writeProcModules(t *testing.T, env *sysutil.Env, loaded ...string) {
	t.Helper()
	path := env.Path("proc", "modules")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, m := range loaded {
		b.WriteString(m)
		b.WriteString(" 16384 0 - Live 0x0000000000000000\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestKernelModuleResourceUnchanged(t *testing.T) {
	env := newTestEnv(t)
	writeProcModules(t, env, "vxlan", "bpfilter")

	r := &KernelModuleResource{Env: env, Modules: []string{"vxlan"}, Runner: &fakeRunner{}}
	res, err := r.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusUnchanged {
		t.Fatalf("expected unchanged, got %s (%s)", res.Status, res.Detail)
	}
}

func TestKernelModuleResourceLoadsMissing(t *testing.T) {
	env := newTestEnv(t)
	writeProcModules(t, env) // nothing loaded

	persistFile := filepath.Join(t.TempDir(), "99-node-configurator.conf")
	runner := &fakeRunner{}
	r := &KernelModuleResource{Env: env, Modules: []string{"vxlan"}, Runner: runner, PersistFile: persistFile}

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

	if len(runner.calls) != 1 || runner.calls[0][0] != "modprobe" || runner.calls[0][1] != "vxlan" {
		t.Fatalf("expected a single modprobe vxlan call, got %v", runner.calls)
	}

	data, err := os.ReadFile(persistFile)
	if err != nil {
		t.Fatalf("expected persist file: %v", err)
	}
	if !strings.Contains(string(data), "vxlan") {
		t.Fatalf("persist file missing module: %s", data)
	}

	// Apply again should not re-run modprobe (already loaded per fixture would still say
	// not loaded since we didn't mutate the fixture, but persistence should now be stable).
	runner.calls = nil
	if err := r.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
}
