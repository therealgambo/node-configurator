package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadMissingFileReturnsZeroValue(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("expected no error for a missing config file, got %v", err)
	}
	if cfg.DryRun || cfg.LogFormat != "" || len(cfg.Resources.Disable) != 0 {
		t.Fatalf("expected zero-value config, got %+v", cfg)
	}
}

func TestLoadMalformedFileErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("dryRun: [this is not a bool"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected an error for malformed YAML")
	}
}

func TestLoadParsesFullExample(t *testing.T) {
	yamlContent := `
dryRun: true
logFormat: json
resources:
  disable: ["cpu-governor", "io-scheduler"]
sysctl:
  overrides:
    net.ipv4.tcp_congestion_control: bbr
limits:
  nofile: 2097152
  nproc: 65536
hugePages:
  enabled: true
  count2M: 512
cilium:
  tunnelMode: vxlan
  legacyIPTables: true
network:
  irqAffinity: false
  ringBufferMax: true
  rps: false
  xps: false
storage:
  ioScheduler: ""
cpu:
  governor: ""
facts:
  cacheTTL: "1h"
`
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !cfg.DryRun || cfg.LogFormat != "json" {
		t.Errorf("unexpected top-level fields: %+v", cfg)
	}
	if len(cfg.Resources.Disable) != 2 || cfg.Resources.Disable[0] != "cpu-governor" {
		t.Errorf("unexpected resources.disable: %v", cfg.Resources.Disable)
	}
	if cfg.Sysctl.Overrides["net.ipv4.tcp_congestion_control"] != "bbr" {
		t.Errorf("unexpected sysctl override: %v", cfg.Sysctl.Overrides)
	}
	if cfg.Limits.NoFile != 2097152 || cfg.Limits.NProc != 65536 {
		t.Errorf("unexpected limits: %+v", cfg.Limits)
	}
	if !cfg.HugePages.Enabled || cfg.HugePages.Count2M != 512 {
		t.Errorf("unexpected hugePages: %+v", cfg.HugePages)
	}
	if cfg.Cilium.TunnelMode != "vxlan" || !cfg.Cilium.LegacyIPTables {
		t.Errorf("unexpected cilium config: %+v", cfg.Cilium)
	}
	if cfg.Network.IRQAffinity == nil || *cfg.Network.IRQAffinity {
		t.Errorf("expected irqAffinity=false, got %+v", cfg.Network.IRQAffinity)
	}
	if cfg.Network.RingBufferMax == nil || !*cfg.Network.RingBufferMax {
		t.Errorf("expected ringBufferMax=true, got %+v", cfg.Network.RingBufferMax)
	}
	if cfg.Storage.IOScheduler == nil || *cfg.Storage.IOScheduler != "" {
		t.Errorf("expected ioScheduler explicit empty string, got %+v", cfg.Storage.IOScheduler)
	}
	if cfg.CPU.Governor == nil || *cfg.CPU.Governor != "" {
		t.Errorf("expected governor explicit empty string, got %+v", cfg.CPU.Governor)
	}
	if time.Duration(cfg.Facts.CacheTTL) != time.Hour {
		t.Errorf("expected cacheTTL=1h, got %v", time.Duration(cfg.Facts.CacheTTL))
	}
}

func TestNetworkTogglesDistinguishUnsetFromFalse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	// network section omitted entirely.
	if err := os.WriteFile(path, []byte("dryRun: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Network.IRQAffinity != nil {
		t.Errorf("expected nil (unset) when network section omitted, got %+v", cfg.Network.IRQAffinity)
	}
	if cfg.CPU.Governor != nil {
		t.Errorf("expected nil (unset) when cpu section omitted, got %+v", cfg.CPU.Governor)
	}
}

func TestToProfileOverrides(t *testing.T) {
	governor := "performance"
	cfg := Config{
		Sysctl:    SysctlConfig{Overrides: map[string]string{"net.core.somaxconn": "1234"}},
		Limits:    LimitsConfig{NoFile: 2097152},
		HugePages: HugePagesConfig{Enabled: true, Count2M: 256},
		Cilium:    CiliumConfig{TunnelMode: "geneve", LegacyIPTables: true},
		CPU:       CPUConfig{Governor: &governor},
	}

	ov := cfg.ToProfileOverrides()
	if ov.SysctlOverrides["net.core.somaxconn"] != "1234" {
		t.Errorf("unexpected sysctl overrides: %v", ov.SysctlOverrides)
	}
	if ov.NoFileLimit != 2097152 {
		t.Errorf("unexpected NoFileLimit: %d", ov.NoFileLimit)
	}
	if ov.HugePages2M != 256 {
		t.Errorf("unexpected HugePages2M: %d", ov.HugePages2M)
	}
	if ov.Cilium.TunnelMode != "geneve" || !ov.Cilium.LegacyIPTables {
		t.Errorf("unexpected cilium options: %+v", ov.Cilium)
	}
	if ov.CPUGovernor == nil || *ov.CPUGovernor != "performance" {
		t.Errorf("unexpected CPUGovernor: %+v", ov.CPUGovernor)
	}
}

func TestToProfileOverridesHugePagesDisabledIgnoresCount(t *testing.T) {
	cfg := Config{HugePages: HugePagesConfig{Enabled: false, Count2M: 999}}
	ov := cfg.ToProfileOverrides()
	if ov.HugePages2M != 0 {
		t.Errorf("expected HugePages2M=0 when disabled regardless of count, got %d", ov.HugePages2M)
	}
}
