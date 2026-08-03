package resource

import (
	"context"
	"fmt"
	"os"

	"github.com/therealgambo/node-configurator/internal/sysutil"
)

// DropInFileResource is a generic "render this exact file, write it only if
// content differs" resource. It backs limits.d, systemd LimitNOFILE
// drop-ins, and anything else that's just a rendered config file rather
// than a live kernel/sysfs knob.
type DropInFileResource struct {
	ResourceName string
	Path         string
	Content      string
	Mode         os.FileMode
}

func (r *DropInFileResource) Name() string { return r.ResourceName }

func (r *DropInFileResource) Check(ctx context.Context) (Result, error) {
	existing, err := sysutil.ReadFileString(r.Path)
	if err == nil && existing == r.Content {
		return Result{Name: r.Name(), Status: StatusUnchanged}, nil
	}
	return Result{Name: r.Name(), Status: StatusWouldChange, Detail: fmt.Sprintf("%s out of date", r.Path)}, nil
}

func (r *DropInFileResource) Apply(ctx context.Context) error {
	mode := r.Mode
	if mode == 0 {
		mode = 0o644
	}
	if _, err := sysutil.WriteFileIfChanged(r.Path, []byte(r.Content), mode); err != nil {
		return fmt.Errorf("writing %s: %w", r.Path, err)
	}
	return nil
}
