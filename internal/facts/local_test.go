package facts

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/therealgambo/node-configurator/internal/sysutil"
)

func TestLocalFallback(t *testing.T) {
	dir := t.TempDir()
	env := &sysutil.Env{Root: dir}

	if err := os.MkdirAll(env.Path("proc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env.Path("proc", "meminfo"), []byte("MemTotal:       16374912 kB\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	netDir := env.Path("sys", "class", "net", "eth0")
	if err := os.MkdirAll(netDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(netDir, "speed"), []byte("10000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	loDir := env.Path("sys", "class", "net", "lo")
	if err := os.MkdirAll(loDir, 0o755); err != nil {
		t.Fatal(err)
	}

	nvmeDir := env.Path("sys", "class", "nvme", "nvme1")
	if err := os.MkdirAll(nvmeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nvmeDir, "model"), []byte("Amazon EC2 NVMe Instance Storage\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	f := LocalFallback(env)

	if f.Source != "local-fallback" {
		t.Fatalf("expected source local-fallback, got %s", f.Source)
	}
	if f.VCPUs <= 0 {
		t.Fatalf("expected positive VCPUs, got %d", f.VCPUs)
	}
	if f.MemMiB != 16374912/1024 {
		t.Fatalf("unexpected MemMiB: %d", f.MemMiB)
	}
	if f.NetworkBandwidthGbps != 10 {
		t.Fatalf("expected 10 Gbps, got %v", f.NetworkBandwidthGbps)
	}
	if !f.NvmeInstanceStore {
		t.Fatal("expected NvmeInstanceStore true when model reports Instance Storage")
	}
}

func TestDetectNvmeInstanceStoreIgnoresEBSVolumes(t *testing.T) {
	dir := t.TempDir()
	env := &sysutil.Env{Root: dir}

	nvmeDir := env.Path("sys", "class", "nvme", "nvme0")
	if err := os.MkdirAll(nvmeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nvmeDir, "model"), []byte("Amazon Elastic Block Store\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if detectNvmeInstanceStore(env) {
		t.Fatal("expected EBS-backed NVMe volume to not be reported as instance store")
	}
}
