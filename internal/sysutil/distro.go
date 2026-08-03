package sysutil

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// Distro identifies the Linux distribution, parsed from /etc/os-release.
type Distro struct {
	ID        string // e.g. "ubuntu", "amzn"
	VersionID string // e.g. "22.04", "2023"
}

// IsAmazonLinux2 reports whether this is Amazon Linux 2 specifically (as
// opposed to AL2023), which matters because AL2 may still default to a
// cgroup v1 hierarchy.
func (d Distro) IsAmazonLinux2() bool {
	return d.ID == "amzn" && strings.HasPrefix(d.VersionID, "2")
}

// IsAmazonLinux2023 reports whether this is Amazon Linux 2023.
func (d Distro) IsAmazonLinux2023() bool {
	return d.ID == "amzn" && strings.HasPrefix(d.VersionID, "2023")
}

// IsUbuntu reports whether this is Ubuntu.
func (d Distro) IsUbuntu() bool {
	return d.ID == "ubuntu"
}

// DetectDistro parses /etc/os-release under e.Root.
func (e *Env) DetectDistro() (Distro, error) {
	f, err := os.Open(e.Path("etc", "os-release"))
	if err != nil {
		return Distro{}, err
	}
	defer func() { _ = f.Close() }()

	d := Distro{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = strings.Trim(value, `"`)
		switch key {
		case "ID":
			d.ID = value
		case "VERSION_ID":
			d.VersionID = value
		}
	}
	return d, scanner.Err()
}

// KernelVersion returns the running kernel release string (e.g.
// "6.1.55-x.amzn2023.x86_64"), read from /proc/sys/kernel/osrelease so it
// respects e.Root the same way the rest of sysutil does (uname(2) cannot be
// re-rooted).
func (e *Env) KernelVersion() (string, error) {
	return e.ReadSysctl("kernel.osrelease")
}

// CgroupV2Unified reports whether the host is running the unified cgroup v2
// hierarchy (cgroup.controllers present at the cgroup mount root). Amazon
// Linux 2 with an old kernel command line may still be on the v1 hybrid
// hierarchy, which requires a reboot with systemd.unified_cgroup_hierarchy=1
// to fix — this function only detects that condition, it does not fix it.
func (e *Env) CgroupV2Unified() bool {
	_, err := os.Stat(e.Path("sys", "fs", "cgroup", "cgroup.controllers"))
	return err == nil
}

// ParseMemInfoTotalKiB extracts MemTotal (in KiB) from /proc/meminfo,
// used as a local fallback when EC2 instance metadata is unavailable.
func (e *Env) ParseMemInfoTotalKiB() (int64, error) {
	f, err := os.Open(e.Path("proc", "meminfo"))
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			return strconv.ParseInt(fields[1], 10, 64)
		}
	}
	return 0, scanner.Err()
}
