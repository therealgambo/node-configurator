package resource

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/therealgambo/node-configurator/internal/sysutil"
)

// MountResource ensures a filesystem is mounted at a path (e.g. bpffs at
// /sys/fs/bpf, required by Cilium) and, optionally, persists an /etc/fstab
// entry so it remounts on reboot without depending on this tool running
// before Cilium starts.
type MountResource struct {
	Env          *sysutil.Env
	Runner       sysutil.Runner
	ResourceName string
	Device       string // e.g. "bpffs" (pseudo-filesystems use their fstype as the device name)
	Path         string // mount point, e.g. /sys/fs/bpf
	FSType       string
	Options      string // mount options, e.g. "rw,nosuid,nodev,noexec,relatime"
	FstabPersist bool
}

func (r *MountResource) Name() string { return r.ResourceName }

func (r *MountResource) Check(ctx context.Context) (Result, error) {
	mounted, err := r.isMounted()
	if err != nil {
		return Result{Name: r.Name(), Status: StatusFailed, Detail: err.Error()}, err
	}

	fstabDrift := false
	if r.FstabPersist {
		existing, _ := sysutil.ReadFileString(r.Env.Path("etc", "fstab"))
		fstabDrift = !fstabLineExists(existing, r.Path)
	}

	if mounted && !fstabDrift {
		return Result{Name: r.Name(), Status: StatusUnchanged}, nil
	}

	detail := ""
	if !mounted {
		detail = fmt.Sprintf("%s not mounted at %s", r.FSType, r.Path)
	}
	if fstabDrift {
		detail = joinDetails(detail, "missing /etc/fstab entry")
	}
	return Result{Name: r.Name(), Status: StatusWouldChange, Detail: detail}, nil
}

func (r *MountResource) Apply(ctx context.Context) error {
	mounted, err := r.isMounted()
	if err != nil {
		return err
	}

	if !mounted {
		if err := os.MkdirAll(r.Path, 0o755); err != nil {
			return fmt.Errorf("creating mount point %s: %w", r.Path, err)
		}
		args := []string{"-t", r.FSType, r.Device, r.Path}
		if r.Options != "" {
			args = append([]string{"-o", r.Options}, args...)
		}
		if _, err := r.Runner.Run(ctx, "mount", args...); err != nil {
			return fmt.Errorf("mounting %s at %s: %w", r.FSType, r.Path, err)
		}
	}

	if r.FstabPersist {
		if err := r.persistFstab(); err != nil {
			return err
		}
	}
	return nil
}

func (r *MountResource) isMounted() (bool, error) {
	f, err := os.Open(r.Env.Path("proc", "mounts"))
	if err != nil {
		return false, fmt.Errorf("reading /proc/mounts: %w", err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 3 && fields[1] == r.Path && fields[2] == r.FSType {
			return true, nil
		}
	}
	return false, scanner.Err()
}

func (r *MountResource) persistFstab() error {
	path := r.Env.Path("etc", "fstab")
	existing, err := sysutil.ReadFileString(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading /etc/fstab: %w", err)
	}
	if fstabLineExists(existing, r.Path) {
		return nil
	}

	if err := sysutil.BackupFile(path); err != nil {
		return fmt.Errorf("backing up /etc/fstab: %w", err)
	}

	content := existing
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += fmt.Sprintf("%s %s %s %s 0 0\n", r.Device, r.Path, r.FSType, r.Options)

	if _, err := sysutil.WriteFileIfChanged(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing /etc/fstab: %w", err)
	}
	return nil
}

// fstabLineExists reports whether fstab content already has a non-comment
// entry whose mount-point field matches path.
func fstabLineExists(fstabContent, path string) bool {
	for line := range strings.SplitSeq(fstabContent, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == path {
			return true
		}
	}
	return false
}
