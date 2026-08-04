# node-configurator

Idempotent kernel, network, and performance tuning for AWS EC2 instances
running Kubernetes with [Cilium](https://cilium.io) in kube-proxy-free eBPF
mode. Runs as a one-off CLI command or as a systemd oneshot unit ordered
before containerd/kubelet/Cilium at boot.

## What it does

Given the detected EC2 instance family/size (vCPUs, memory, network
bandwidth tier, storage), it converges the host towards a computed desired
state:

- **sysctl**: file-descriptor/thread ceilings, neighbor-table sizing,
  network buffer sizing, BPF JIT limits — all scaled to instance size, not
  one static file
- **kernel modules**: only what kube-proxy-free Cilium eBPF actually needs
  (no `nf_conntrack`/`br_netfilter`/`ip_vs` by default)
- **ulimits**: `/etc/security/limits.d` plus systemd `LimitNOFILE` drop-ins
  for containerd/kubelet/cilium (PAM limits don't apply to systemd-started
  services)
- **swap**: forced off (`swapoff`, `/etc/fstab`, masks `.swap` units)
- **mounts**: `bpffs` at `/sys/fs/bpf` for Cilium
- **NIC tuning**: ENA ring-buffer maximization, RPS/XPS queue spreading,
  IRQ affinity spread across vCPUs
- **CPU governor**: `performance` where the platform exposes one
- **I/O scheduler**: `none` on NVMe block devices
- **hugepages**: 2M pages via `vm.nr_hugepages` (opt-in, disabled by
  default)

Every resource is idempotent: it reads live state, compares against desired
state, and only writes when they differ. Desired state is also persisted to
standard drop-in locations (`sysctl.d`, `modules-load.d`, `limits.d`,
`fstab`, systemd drop-ins) so it survives reboots independent of whether
this tool's own unit runs again.

## Non-goals

- Not a general config-management tool — no package installation, no
  cloud-init replacement
- Does not create filesystems on local NVMe instance store, only tunes the
  I/O scheduler on block devices that already exist
- Does not manage 1G hugepages or the cgroup v1→v2 migration — both need a
  kernel command-line change and a reboot, which this tool never does
  (see `cgroup-v2` resource: it reports `reboot_required` instead)
- Does not manage Cilium/kubelet/containerd configuration itself, only the
  host-level prerequisites they need
- No kube-proxy/iptables/IPVS support — this fleet runs Cilium kube-proxy-free

## Requirements

- Linux (Ubuntu or Amazon Linux 2/2023); other distros likely work since
  almost everything targets `/proc`, `/sys`, and standard drop-in
  directories directly, but are untested
- For full instance-aware tuning, the instance's IAM role needs
  `ec2:DescribeInstanceTypes`. Without it, node-configurator still runs —
  it falls back to locally-observable facts (`nproc`, `/proc/meminfo`,
  link speed, NVMe model strings) and logs a warning that tuning is
  approximate

## Install

```sh
make build            # cross-compiles bin/node-configurator-linux-{amd64,arm64}
```

Ship the binary matching each instance's architecture plus
`systemd/node-configurator.service` and `configs/example.yaml` via your
existing AMI-build or user-data pipeline, e.g.:

```sh
install -m 0755 bin/node-configurator-linux-amd64 /usr/local/bin/node-configurator
install -m 0644 systemd/node-configurator.service /etc/systemd/system/
mkdir -p /etc/node-configurator
install -m 0644 configs/example.yaml /etc/node-configurator/config.yaml   # edit as needed
systemctl daemon-reload
systemctl enable --now node-configurator.service
```

## CLI

```
node-configurator apply   [flags]   converge the host to desired state
node-configurator check   [flags]   report drift without changing anything (exit 2 if changes are pending)
node-configurator facts   [flags]   print gathered instance/host facts as JSON
node-configurator profile [flags]   print the resolved desired-state profile as JSON
node-configurator version           print the build version
```

Flags: `-config` (default `/etc/node-configurator/config.yaml`),
`-log-format text|json`, `-refresh-facts` (bypass the facts cache),
`-only`/`-skip` (comma-separated resource names).

Exit codes follow the Puppet/Ansible check-mode convention: `0` nothing to
do or everything converged, `1` a resource failed, `2` (`check` only)
changes are pending.

## Configuration

All tuning is computed automatically from the detected instance; nothing in
`configs/example.yaml` is required. Use it to override specific values
(disable a resource, add a sysctl, pick a non-default Cilium tunnel mode)
— see the comments in that file for the full schema.

## Testing this change

This was developed and unit-tested on macOS, which has no `/proc`, `/sys`,
or kernel modules — every OS-touching primitive in `internal/sysutil` and
every `resource.Resource` is rooted at a configurable path so tests exercise
the real logic against a fake filesystem tree (`t.TempDir()`), and external
commands (`modprobe`, `ethtool`, `swapoff`, `systemctl`) go through an
injectable `Runner` interface faked in tests. Run the full suite with:

```sh
go test ./...
```

That gives strong confidence in the decision logic (sysctl scaling
formulas across representative instance types, merge/override precedence,
idempotency of every resource, EC2 API response mapping) but **cannot**
exercise real `/proc/sys` writes, real `modprobe`/`ethtool` invocations, or
real IMDS/EC2 API calls from macOS.

CI's `smoke-test` job (`.github/workflows/ci.yml`) closes part of that gap:
GitHub-hosted Linux runners are real, ephemeral VMs (not containers), so it
builds the binary natively and runs `apply` for real against the runner's
live `/proc`, `/sys`, `/etc`, and systemd — then runs `check` again
afterwards and fails the job if anything still reports drift, catching
idempotency bugs the fake-filesystem unit tests can't. It still can't
exercise ENA/NVMe hardware, real IMDS, or instance-family-aware sizing —
only a real EC2 instance can. Before rolling out to production:

1. Run `node-configurator check` on a real EC2 instance of each family you
   care about and review the report
2. Run `node-configurator apply` on a single canary instance and confirm
   `kubectl get nodes`/pod scheduling/Cilium status are unaffected
3. Re-run `apply` a second time and confirm every resource reports
   `unchanged` (idempotency)
4. Reboot the canary and confirm the systemd unit re-applies cleanly before
   containerd/kubelet/Cilium start

## Package layout

```
cmd/node-configurator/   CLI entrypoint
internal/sysutil/        OS primitives (rooted filesystem, /proc/sys, distro/kernel detection, command execution)
internal/facts/           instance/host discovery: IMDSv2 + EC2 DescribeInstanceTypes, local fallback, disk cache
internal/profile/         desired-state computation: baseline + size/bandwidth scaling + Cilium prerequisites + operator overrides
internal/config/          operator YAML config loader
internal/resource/         the idempotent Check/Apply unit — generic (sysctl, kernel modules, drop-in files, mounts) and bespoke (swap, IRQ affinity, NIC tuning, CPU governor, I/O scheduler, cgroup v2 check) resources
internal/engine/           wires a Profile into concrete resources, runs them, reports results and exit codes
systemd/                   the systemd unit
configs/                   example operator config
```
