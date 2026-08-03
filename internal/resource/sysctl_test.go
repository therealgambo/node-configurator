package resource

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/therealgambo/node-configurator/internal/sysutil"
)

func newTestEnv(t *testing.T) *sysutil.Env {
	t.Helper()
	return &sysutil.Env{Root: t.TempDir()}
}

func writeSysctlFixture(t *testing.T, env *sysutil.Env, key, value string) {
	t.Helper()
	path := env.SysctlPath(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSysctlResourceUnchanged(t *testing.T) {
	env := newTestEnv(t)
	writeSysctlFixture(t, env, "net.core.somaxconn", "32768")

	r := &SysctlResource{
		Env:      env,
		Settings: map[string]string{"net.core.somaxconn": "32768"},
	}

	res, err := r.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusUnchanged {
		t.Fatalf("expected unchanged, got %s (%s)", res.Status, res.Detail)
	}
}

func TestSysctlResourceDriftAndApply(t *testing.T) {
	env := newTestEnv(t)
	writeSysctlFixture(t, env, "net.core.somaxconn", "128")

	persistFile := filepath.Join(t.TempDir(), "99-node-configurator.conf")
	r := &SysctlResource{
		Env:         env,
		Settings:    map[string]string{"net.core.somaxconn": "32768"},
		PersistFile: persistFile,
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

	got, err := env.ReadSysctl("net.core.somaxconn")
	if err != nil {
		t.Fatal(err)
	}
	if got != "32768" {
		t.Fatalf("expected live value 32768, got %s", got)
	}

	data, err := os.ReadFile(persistFile)
	if err != nil {
		t.Fatalf("expected persist file to be written: %v", err)
	}
	if !strings.Contains(string(data), "net.core.somaxconn = 32768") {
		t.Fatalf("persist file missing expected line: %s", data)
	}

	// Second Check should now report unchanged (idempotency).
	res, err = r.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusUnchanged {
		t.Fatalf("expected unchanged after apply, got %s (%s)", res.Status, res.Detail)
	}
}

// TestSysctlResourceToleratesKernelWhitespaceReformatting reproduces a real
// bug found on a live kernel: multi-value sysctls like net.ipv4.tcp_rmem
// are always echoed back tab-separated on read, regardless of how they
// were written (we render them space-separated) -- a byte-exact comparison
// reported permanent drift immediately after a successful apply.
func TestSysctlResourceToleratesKernelWhitespaceReformatting(t *testing.T) {
	env := newTestEnv(t)
	writeSysctlFixture(t, env, "net.ipv4.tcp_rmem", "4096\t131072\t33554432")

	r := &SysctlResource{
		Env:      env,
		Settings: map[string]string{"net.ipv4.tcp_rmem": "4096 131072 33554432"},
	}

	res, err := r.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusUnchanged {
		t.Fatalf("expected unchanged (whitespace-only difference from kernel reformatting), got %s (%s)", res.Status, res.Detail)
	}
}

func TestNormalizeSysctlValue(t *testing.T) {
	cases := map[string]string{
		"4096\t131072\t33554432":      "4096 131072 33554432",
		"4096 131072 33554432":        "4096 131072 33554432",
		"  4096   131072  33554432  ": "4096 131072 33554432",
		"32768":                       "32768",
	}
	for in, want := range cases {
		if got := normalizeSysctlValue(in); got != want {
			t.Errorf("normalizeSysctlValue(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSysctlResourceSkipsMissingKernelKey(t *testing.T) {
	env := newTestEnv(t)
	// net.core.somaxconn does not exist in the fixture at all.
	r := &SysctlResource{
		Env:      env,
		Settings: map[string]string{"net.core.somaxconn": "32768"},
	}

	res, err := r.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusUnchanged {
		t.Fatalf("expected unchanged (skipped), got %s", res.Status)
	}
	if !strings.Contains(res.Detail, "skipped") {
		t.Fatalf("expected detail to mention skipped key, got %q", res.Detail)
	}
}
