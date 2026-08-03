// Package facts discovers what kind of EC2 instance node-configurator is
// running on: its family/size (vCPUs, memory, network bandwidth tier,
// storage), architecture, distro, and kernel — the inputs the profile
// package uses to compute desired tuning state. AWS instance-type facts are
// gathered via aws-sdk-go-v2 (IMDSv2 + EC2 DescribeInstanceTypes) and cached
// locally; if the EC2 API is unreachable or unauthorized, Gather degrades to
// locally-observable facts rather than failing outright.
package facts

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/therealgambo/node-configurator/internal/sysutil"
)

// Facts describes the instance and host node-configurator is tuning.
type Facts struct {
	// Identity (from IMDS)
	InstanceID       string `json:"instanceId"`
	InstanceType     string `json:"instanceType"`
	Family           string `json:"family"` // instance-type prefix (e.g. "m6i"), for logging/cache keys only — NOT used for tuning decisions
	Region           string `json:"region"`
	AvailabilityZone string `json:"availabilityZone"`
	Arch             string `json:"arch"` // "x86_64" or "arm64"

	// Sizing (from EC2 DescribeInstanceTypes, or local fallback)
	VCPUs                int     `json:"vcpus"`
	MemMiB               int64   `json:"memMiB"`
	NetworkBandwidthGbps float64 `json:"networkBandwidthGbps"` // baseline bandwidth of the default network card, best-effort
	EnaSupport           bool    `json:"enaSupport"`
	MaxENIs              int     `json:"maxENIs"`
	Ipv4AddressesPerENI  int     `json:"ipv4AddressesPerENI"`
	NvmeInstanceStore    bool    `json:"nvmeInstanceStore"`
	BareMetal            bool    `json:"bareMetal"`
	Hypervisor           string  `json:"hypervisor"` // "nitro" | "xen" | ""

	// Host (always gathered locally, never cached from a previous boot's OS)
	Distro        sysutil.Distro `json:"distro"`
	KernelVersion string         `json:"kernelVersion"`

	// Provenance
	Source   string   `json:"source"` // "ec2-api" | "local-fallback"
	Warnings []string `json:"warnings,omitempty"`
}

// MaxNeighborEntries is a rough upper bound on L2 neighbor-table entries this
// node might need (ENIs * IPs-per-ENI), used to size net.ipv4.neigh.*
// gc_thresh sysctls. It intentionally over-estimates: undersizing the
// neighbor table causes silent packet loss under pod churn, which is a worse
// failure mode than a slightly larger table.
func (f Facts) MaxNeighborEntries() int {
	enis, ips := f.MaxENIs, f.Ipv4AddressesPerENI
	if enis <= 0 {
		enis = 1
	}
	if ips <= 0 {
		ips = 1
	}
	return enis * ips
}

// Gatherer collects Facts, preferring the EC2 API and falling back to local
// observation when it's unavailable.
type Gatherer struct {
	Env       *sysutil.Env
	CachePath string        // defaults to DefaultCachePath
	CacheTTL  time.Duration // 0 = cached facts never expire (keyed by instance ID)
	Refresh   bool          // bypass the cache
}

func (g *Gatherer) env() *sysutil.Env {
	if g.Env != nil {
		return g.Env
	}
	return sysutil.New()
}

func (g *Gatherer) cachePath() string {
	if g.CachePath != "" {
		return g.CachePath
	}
	return DefaultCachePath
}

// Gather resolves Facts for the current host. It never returns an error for
// degraded-but-usable conditions (no IMDS, no EC2 permission) — those are
// reported via Facts.Warnings and Facts.Source instead, so a systemd unit
// running node-configurator doesn't fail boot just because the EC2 API was
// briefly unreachable.
func (g *Gatherer) Gather(ctx context.Context) (Facts, error) {
	env := g.env()
	distro, _ := env.DetectDistro()
	kernel, _ := env.KernelVersion()

	identity, err := gatherIdentity(ctx)
	if err != nil {
		f := LocalFallback(env)
		f.Distro, f.KernelVersion = distro, kernel
		f.Warnings = append(f.Warnings, fmt.Sprintf("IMDS unavailable, using local-only facts: %v", err))
		return f, nil
	}
	identity.Family = familyFromInstanceType(identity.InstanceType)

	if !g.Refresh {
		if cached, ok := loadCache(env, g.cachePath(), identity.InstanceID, g.CacheTTL); ok {
			cached.Distro, cached.KernelVersion = distro, kernel
			return cached, nil
		}
	}

	f := identity
	enrichment, err := gatherInstanceTypeInfo(ctx, identity.Region, identity.InstanceType)
	if err != nil {
		local := LocalFallback(env)
		f.VCPUs, f.MemMiB = local.VCPUs, local.MemMiB
		f.NetworkBandwidthGbps, f.NvmeInstanceStore = local.NetworkBandwidthGbps, local.NvmeInstanceStore
		f.Source = "local-fallback"
		f.Distro, f.KernelVersion = distro, kernel
		f.Warnings = append(f.Warnings, fmt.Sprintf("EC2 DescribeInstanceTypes unavailable, family-specific tuning is approximate (check ec2:DescribeInstanceTypes IAM permission): %v", err))
		return f, nil // degraded result: don't cache it, retry the API next run
	}

	f.VCPUs = enrichment.VCPUs
	f.MemMiB = enrichment.MemMiB
	f.NetworkBandwidthGbps = enrichment.NetworkBandwidthGbps
	f.EnaSupport = enrichment.EnaSupport
	f.MaxENIs = enrichment.MaxENIs
	f.Ipv4AddressesPerENI = enrichment.Ipv4AddressesPerENI
	f.NvmeInstanceStore = enrichment.NvmeInstanceStore
	f.BareMetal = enrichment.BareMetal
	f.Hypervisor = enrichment.Hypervisor
	f.Source = "ec2-api"
	f.Distro, f.KernelVersion = distro, kernel

	if err := saveCache(env, g.cachePath(), f); err != nil {
		f.Warnings = append(f.Warnings, fmt.Sprintf("caching facts: %v", err))
	}
	return f, nil
}

// familyFromInstanceType extracts the instance-type prefix ("m6i" from
// "m6i.2xlarge") for logging/cache-keying only.
func familyFromInstanceType(instanceType string) string {
	family, _, _ := strings.Cut(instanceType, ".")
	return family
}
