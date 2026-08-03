package resource

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/therealgambo/node-configurator/internal/sysutil"
)

// IOSchedulerResource sets the block-device I/O scheduler on NVMe devices
// (both EBS-backed and local instance-store) to the configured scheduler
// (typically "none"): the virtualized NVMe controller's own multi-queue
// handling makes an additional elevator scheduler counter-productive.
//
// Only whole-disk devices are targeted (nvme0n1, not partitions like
// nvme0n1p1) since the scheduler is a property of the request queue, which
// partitions share with their parent device.
type IOSchedulerResource struct {
	Env       *sysutil.Env
	Scheduler string // e.g. "none"; empty disables this resource
}

func (r *IOSchedulerResource) Name() string { return "io-scheduler" }

var nvmeWholeDiskRe = regexp.MustCompile(`^nvme\d+n\d+$`)

func (r *IOSchedulerResource) Check(ctx context.Context) (Result, error) {
	if r.Scheduler == "" {
		return Result{Name: r.Name(), Status: StatusSkipped, Detail: "disabled"}, nil
	}

	devices, err := r.nvmeDevices()
	if err != nil {
		return Result{Name: r.Name(), Status: StatusFailed, Detail: err.Error()}, err
	}
	if len(devices) == 0 {
		return Result{Name: r.Name(), Status: StatusSkipped, Detail: "no NVMe block devices found"}, nil
	}

	var drift []string
	for _, dev := range devices {
		cur, available, err := r.readScheduler(dev)
		if err != nil {
			continue
		}
		if cur == r.Scheduler {
			continue
		}
		if !slices.Contains(available, r.Scheduler) {
			continue // not offered by this device's driver; nothing we can do
		}
		drift = append(drift, fmt.Sprintf("%s: %s -> %s", dev, cur, r.Scheduler))
	}

	if len(drift) == 0 {
		return Result{Name: r.Name(), Status: StatusUnchanged}, nil
	}
	return Result{Name: r.Name(), Status: StatusWouldChange, Detail: strings.Join(drift, ", ")}, nil
}

func (r *IOSchedulerResource) Apply(ctx context.Context) error {
	if r.Scheduler == "" {
		return nil
	}
	devices, err := r.nvmeDevices()
	if err != nil {
		return err
	}
	for _, dev := range devices {
		cur, available, err := r.readScheduler(dev)
		if err != nil || cur == r.Scheduler || !slices.Contains(available, r.Scheduler) {
			continue
		}
		if err := os.WriteFile(r.schedulerPath(dev), []byte(r.Scheduler), 0o644); err != nil {
			return fmt.Errorf("setting scheduler for %s: %w", dev, err)
		}
	}
	return nil
}

func (r *IOSchedulerResource) nvmeDevices() ([]string, error) {
	entries, err := os.ReadDir(r.Env.Path("sys", "block"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading /sys/block: %w", err)
	}
	var devices []string
	for _, e := range entries {
		if nvmeWholeDiskRe.MatchString(e.Name()) {
			devices = append(devices, e.Name())
		}
	}
	return devices, nil
}

func (r *IOSchedulerResource) schedulerPath(dev string) string {
	return r.Env.Path("sys", "block", dev, "queue", "scheduler")
}

// readScheduler parses the kernel's scheduler sysfs format:
// "mq-deadline [kyber] none" — the active scheduler is bracketed.
func (r *IOSchedulerResource) readScheduler(dev string) (current string, available []string, err error) {
	raw, err := sysutil.ReadFileString(r.schedulerPath(dev))
	if err != nil {
		return "", nil, err
	}
	for field := range strings.FieldsSeq(strings.TrimSpace(raw)) {
		if strings.HasPrefix(field, "[") && strings.HasSuffix(field, "]") {
			name := strings.Trim(field, "[]")
			current = name
			available = append(available, name)
		} else {
			available = append(available, field)
		}
	}
	return current, available, nil
}
