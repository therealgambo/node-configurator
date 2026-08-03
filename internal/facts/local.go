package facts

import (
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/therealgambo/node-configurator/internal/sysutil"
)

// LocalFallback derives best-effort Facts purely from the running kernel,
// used when IMDS is unreachable or the instance role lacks
// ec2:DescribeInstanceTypes. It cannot determine ENI/IP limits (those are
// EC2-instance-type metadata, not observable locally), only what the kernel
// itself reports.
func LocalFallback(env *sysutil.Env) Facts {
	f := Facts{Source: "local-fallback"}

	f.VCPUs = runtime.NumCPU()
	f.Arch = normalizeArch(runtime.GOARCH)

	if kib, err := env.ParseMemInfoTotalKiB(); err == nil {
		f.MemMiB = kib / 1024
	}

	f.NetworkBandwidthGbps = primaryLinkSpeedGbps(env)
	f.NvmeInstanceStore = detectNvmeInstanceStore(env)

	return f
}

func normalizeArch(goarch string) string {
	switch goarch {
	case "amd64":
		return "x86_64"
	default:
		return goarch // "arm64" already matches EC2/IMDS convention
	}
}

// primaryLinkSpeedGbps reads the link speed (Mbps) reported by the driver
// for the primary interface, as a rough stand-in for the instance's network
// bandwidth tier.
func primaryLinkSpeedGbps(env *sysutil.Env) float64 {
	iface := env.PrimaryInterface()
	if iface == "" {
		return 0
	}
	data, err := os.ReadFile(env.Path("sys", "class", "net", iface, "speed"))
	if err != nil {
		return 0
	}
	mbps, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || mbps <= 0 {
		return 0
	}
	return float64(mbps) / 1000.0
}

var nvmeInstanceStoreModelRe = regexp.MustCompile(`(?i)instance storage`)

// detectNvmeInstanceStore distinguishes local NVMe instance store from an
// EBS-backed NVMe root/data volume by reading the controller's reported
// model string. On Nitro instances both show up identically as /dev/nvmeNn1
// block devices, so presence alone (which basically every modern instance
// has, via its EBS root volume) is not a usable signal.
func detectNvmeInstanceStore(env *sysutil.Env) bool {
	controllers, err := os.ReadDir(env.Path("sys", "class", "nvme"))
	if err != nil {
		return false
	}
	for _, c := range controllers {
		model, err := os.ReadFile(env.Path("sys", "class", "nvme", c.Name(), "model"))
		if err != nil {
			continue
		}
		if nvmeInstanceStoreModelRe.Match(model) {
			return true
		}
	}
	return false
}
