package resource

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/therealgambo/node-configurator/internal/sysutil"
)

// NICResource tunes the primary ENA interface: maximizes ring buffer sizes
// (within the driver's own advertised limits), and spreads packet
// processing across vCPUs via RPS/XPS when hardware queue count is lower
// than vCPU count (common on smaller instance sizes).
type NICResource struct {
	Env           *sysutil.Env
	Runner        sysutil.Runner
	Interface     string
	NumCPUs       int
	RingBufferMax bool
	RPS           bool
	XPS           bool
}

func (r *NICResource) Name() string { return "nic-tuning" }

func (r *NICResource) Check(ctx context.Context) (Result, error) {
	if r.Interface == "" {
		return Result{Name: r.Name(), Status: StatusSkipped, Detail: "no interface detected"}, nil
	}

	var drift []string

	if r.RingBufferMax {
		// ethtool absent or interface doesn't support ring queries (e.g. some
		// virtio/older drivers) is treated as skipped, not a failure.
		if ringDrift, err := r.ringBufferDrift(ctx); err == nil && ringDrift != "" {
			drift = append(drift, ringDrift)
		}
	}

	if r.RPS && r.NumCPUs > 1 {
		queueDrift, err := r.queueMaskDrift("rx", "rps_cpus")
		if err == nil && len(queueDrift) > 0 {
			drift = append(drift, fmt.Sprintf("rps_cpus out of date on %d queue(s)", len(queueDrift)))
		}
	}

	if r.XPS && r.NumCPUs > 1 {
		queueDrift, err := r.queueMaskDrift("tx", "xps_cpus")
		if err == nil && len(queueDrift) > 0 {
			drift = append(drift, fmt.Sprintf("xps_cpus out of date on %d queue(s)", len(queueDrift)))
		}
	}

	if len(drift) == 0 {
		return Result{Name: r.Name(), Status: StatusUnchanged}, nil
	}
	return Result{Name: r.Name(), Status: StatusWouldChange, Detail: strings.Join(drift, "; ")}, nil
}

func (r *NICResource) Apply(ctx context.Context) error {
	if r.Interface == "" {
		return nil
	}

	if r.RingBufferMax {
		if err := r.applyRingBufferMax(ctx); err != nil {
			return err
		}
	}
	if r.RPS && r.NumCPUs > 1 {
		if err := r.applyQueueMask("rx", "rps_cpus"); err != nil {
			return err
		}
	}
	if r.XPS && r.NumCPUs > 1 {
		if err := r.applyQueueMask("tx", "xps_cpus"); err != nil {
			return err
		}
	}
	return nil
}

// ringParams holds the parsed "Pre-set maximums" and "Current hardware
// settings" RX/TX values from `ethtool -g <iface>` output.
type ringParams struct {
	maxRX, maxTX, curRX, curTX int
}

var ethtoolRingLineRe = regexp.MustCompile(`^(RX|TX):\s+(\d+|n/a)`)

func parseEthtoolRingParams(output string) ringParams {
	var rp ringParams
	section := ""
	for line := range strings.SplitSeq(output, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "Pre-set maximums"):
			section = "max"
			continue
		case strings.HasPrefix(trimmed, "Current hardware settings"):
			section = "current"
			continue
		}
		m := ethtoolRingLineRe.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}
		v, err := strconv.Atoi(m[2])
		if err != nil {
			continue // "n/a": this direction isn't supported by the driver
		}
		switch {
		case section == "max" && m[1] == "RX":
			rp.maxRX = v
		case section == "max" && m[1] == "TX":
			rp.maxTX = v
		case section == "current" && m[1] == "RX":
			rp.curRX = v
		case section == "current" && m[1] == "TX":
			rp.curTX = v
		}
	}
	return rp
}

func (r *NICResource) ringBufferDrift(ctx context.Context) (string, error) {
	out, err := r.Runner.Run(ctx, "ethtool", "-g", r.Interface)
	if err != nil {
		return "", fmt.Errorf("ethtool -g %s: %w", r.Interface, err)
	}
	rp := parseEthtoolRingParams(out)
	var drift []string
	if rp.maxRX > 0 && rp.curRX < rp.maxRX {
		drift = append(drift, fmt.Sprintf("rx %d -> %d", rp.curRX, rp.maxRX))
	}
	if rp.maxTX > 0 && rp.curTX < rp.maxTX {
		drift = append(drift, fmt.Sprintf("tx %d -> %d", rp.curTX, rp.maxTX))
	}
	if len(drift) == 0 {
		return "", nil
	}
	return "ring buffers: " + strings.Join(drift, ", "), nil
}

func (r *NICResource) applyRingBufferMax(ctx context.Context) error {
	out, err := r.Runner.Run(ctx, "ethtool", "-g", r.Interface)
	if err != nil {
		// Same tolerance as ringBufferDrift in Check: ethtool missing, or
		// the driver not supporting ring queries, isn't a failure -- it's
		// this NIC not offering the feature. Apply is only reached because
		// *some* NIC feature (possibly RPS/XPS, not ring buffers) drifted,
		// so this step must degrade gracefully rather than failing the
		// whole resource.
		return nil
	}
	rp := parseEthtoolRingParams(out)

	var args []string
	if rp.maxRX > 0 && rp.curRX < rp.maxRX {
		args = append(args, "rx", strconv.Itoa(rp.maxRX))
	}
	if rp.maxTX > 0 && rp.curTX < rp.maxTX {
		args = append(args, "tx", strconv.Itoa(rp.maxTX))
	}
	if len(args) == 0 {
		return nil
	}
	if _, err := r.Runner.Run(ctx, "ethtool", append([]string{"-G", r.Interface}, args...)...); err != nil {
		return fmt.Errorf("ethtool -G %s: %w", r.Interface, err)
	}
	return nil
}

func (r *NICResource) queueDirs(direction string) ([]string, error) {
	pattern := r.Env.Path("sys", "class", "net", r.Interface, "queues", direction+"-*")
	return filepath.Glob(pattern)
}

func (r *NICResource) queueMaskDrift(direction, attr string) ([]string, error) {
	dirs, err := r.queueDirs(direction)
	if err != nil {
		return nil, err
	}
	want := cpumaskAll(r.NumCPUs)
	var drifted []string
	for _, dir := range dirs {
		path := filepath.Join(dir, attr)
		cur, err := sysutil.ReadFileString(path)
		if err != nil {
			continue
		}
		if strings.TrimSpace(cur) != want {
			drifted = append(drifted, path)
		}
	}
	return drifted, nil
}

func (r *NICResource) applyQueueMask(direction, attr string) error {
	dirs, err := r.queueDirs(direction)
	if err != nil {
		return err
	}
	want := cpumaskAll(r.NumCPUs)
	for _, dir := range dirs {
		path := filepath.Join(dir, attr)
		cur, err := sysutil.ReadFileString(path)
		if err == nil && strings.TrimSpace(cur) == want {
			continue
		}
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			if os.IsNotExist(err) {
				// This queue directory exists but doesn't expose this
				// particular steering attribute (seen on e.g. a bridge
				// interface's queues, which have some but not all of the
				// files a real multi-queue NIC's queues have) -- skip it
				// rather than failing every other queue behind it.
				continue
			}
			return fmt.Errorf("writing %s: %w", path, err)
		}
	}
	return nil
}

// cpumaskAll renders a kernel cpumask_list-style bitmap ("ffffffff,0000000f")
// with the low n bits set, one bit per CPU 0..n-1, matching how the kernel
// expects rps_cpus/xps_cpus to be written (comma-separated 32-bit hex
// words, most-significant word first).
func cpumaskAll(n int) string {
	if n <= 0 {
		n = 1
	}
	numWords := (n + 31) / 32
	words := make([]string, numWords)
	remaining := n
	for i := range numWords {
		bits := min(remaining, 32)
		var mask uint32
		if bits >= 32 {
			mask = 0xffffffff
		} else if bits > 0 {
			mask = (uint32(1) << uint(bits)) - 1
		}
		words[i] = fmt.Sprintf("%08x", mask)
		remaining -= bits
	}
	for l, rr := 0, len(words)-1; l < rr; l, rr = l+1, rr-1 {
		words[l], words[rr] = words[rr], words[l]
	}
	return strings.Join(words, ",")
}
