// Package config loads the operator's optional YAML override file. Every
// field is optional; an absent file, or an absent field within a present
// file, means "use node-configurator's computed default for this instance."
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/therealgambo/node-configurator/internal/profile"
)

// Config is the parsed operator YAML file (see configs/example.yaml).
type Config struct {
	DryRun    bool            `yaml:"dryRun"`
	LogFormat string          `yaml:"logFormat"` // "text" (default) or "json"
	Resources ResourcesConfig `yaml:"resources"`
	Sysctl    SysctlConfig    `yaml:"sysctl"`
	Limits    LimitsConfig    `yaml:"limits"`
	HugePages HugePagesConfig `yaml:"hugePages"`
	Cilium    CiliumConfig    `yaml:"cilium"`
	Network   NetworkConfig   `yaml:"network"`
	Storage   StorageConfig   `yaml:"storage"`
	CPU       CPUConfig       `yaml:"cpu"`
	Facts     FactsConfig     `yaml:"facts"`
}

// ResourcesConfig lets an operator skip a resource entirely by its
// Resource.Name(), e.g. ["cpu-governor"].
type ResourcesConfig struct {
	Disable []string `yaml:"disable"`
}

type SysctlConfig struct {
	Overrides map[string]string `yaml:"overrides"`
}

type LimitsConfig struct {
	NoFile uint64 `yaml:"nofile"`
	NProc  uint64 `yaml:"nproc"`
}

type HugePagesConfig struct {
	Enabled bool `yaml:"enabled"`
	Count2M int  `yaml:"count2M"`
}

// CiliumConfig controls the host prerequisites prepared for Cilium. See
// profile.CiliumOptions for the defaults these override.
type CiliumConfig struct {
	TunnelMode     string `yaml:"tunnelMode"`
	LegacyIPTables bool   `yaml:"legacyIPTables"`
}

// NetworkConfig toggles individual features inside the NIC/IRQ resources.
// Pointers distinguish "not set in YAML" (nil, keep computed default) from
// an explicit true/false.
type NetworkConfig struct {
	IRQAffinity   *bool `yaml:"irqAffinity"`
	RingBufferMax *bool `yaml:"ringBufferMax"`
	RPS           *bool `yaml:"rps"`
	XPS           *bool `yaml:"xps"`
}

// StorageConfig controls the NVMe I/O scheduler resource. nil keeps the
// computed default ("none"); an explicit "" disables the resource.
type StorageConfig struct {
	IOScheduler *string `yaml:"ioScheduler"`
}

// CPUConfig controls the CPU governor resource. nil keeps the computed
// default ("performance"); an explicit "" disables the resource.
type CPUConfig struct {
	Governor *string `yaml:"governor"`
}

// FactsConfig controls how instance facts are gathered/cached.
type FactsConfig struct {
	// CacheTTL as a Go duration string (e.g. "1h"). Zero/absent means
	// cached facts never expire (they're keyed by instance ID and an
	// instance's type/vCPUs/memory/network don't change during its
	// lifetime barring a stop/modify/start cycle).
	CacheTTL Duration `yaml:"cacheTTL"`
}

// Duration wraps time.Duration so it can be written as a YAML string
// ("1h", "30m") instead of a raw integer nanosecond count.
type Duration time.Duration

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	if s == "" {
		*d = 0
		return nil
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(parsed)
	return nil
}

// Load reads and parses path. A missing file is not an error — it just
// means every field falls back to its computed default — but a malformed
// one is.
func Load(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("reading config %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing config %s: %w", path, err)
	}
	return cfg, nil
}

// ToProfileOverrides converts the parsed config into profile.Overrides.
func (c Config) ToProfileOverrides() profile.Overrides {
	var hugePages2M int
	if c.HugePages.Enabled {
		hugePages2M = c.HugePages.Count2M
	}

	return profile.Overrides{
		SysctlOverrides: c.Sysctl.Overrides,
		NoFileLimit:     c.Limits.NoFile,
		CPUGovernor:     c.CPU.Governor,
		HugePages2M:     hugePages2M,
		Cilium: profile.CiliumOptions{
			TunnelMode:     c.Cilium.TunnelMode,
			LegacyIPTables: c.Cilium.LegacyIPTables,
		},
		IRQAffinity:   c.Network.IRQAffinity,
		RingBufferMax: c.Network.RingBufferMax,
		RPS:           c.Network.RPS,
		XPS:           c.Network.XPS,
		IOScheduler:   c.Storage.IOScheduler,
	}
}
