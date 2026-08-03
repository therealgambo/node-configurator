package resource

import (
	"context"

	"github.com/therealgambo/node-configurator/internal/sysutil"
)

// CgroupV2Resource verifies the host is on the unified cgroup v2 hierarchy,
// which Cilium's socket-based load-balancing (kube-proxy replacement)
// requires. Amazon Linux 2 with an older kernel command line can still boot
// into the cgroup v1 hybrid hierarchy; fixing that needs
// systemd.unified_cgroup_hierarchy=1 on the kernel command line and a
// reboot, which this tool deliberately never does. Check reports
// StatusRebootRequired instead of failing so it's visible in the run report
// without blocking every other resource.
type CgroupV2Resource struct {
	Env *sysutil.Env
}

func (r *CgroupV2Resource) Name() string { return "cgroup-v2" }

func (r *CgroupV2Resource) Check(ctx context.Context) (Result, error) {
	if r.Env.CgroupV2Unified() {
		return Result{Name: r.Name(), Status: StatusUnchanged}, nil
	}
	return Result{
		Name:   r.Name(),
		Status: StatusRebootRequired,
		Detail: "host is on the cgroup v1 hierarchy; add systemd.unified_cgroup_hierarchy=1 to the kernel command line and reboot",
	}, nil
}

// Apply is a deliberate no-op: reaching cgroup v2 requires a kernel
// command-line change and reboot, which is out of scope for a tool that
// never reboots the node.
func (r *CgroupV2Resource) Apply(ctx context.Context) error {
	return nil
}
