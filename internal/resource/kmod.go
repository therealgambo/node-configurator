package resource

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/therealgambo/node-configurator/internal/sysutil"
)

// KernelModuleResource ensures a set of kernel modules is loaded now and
// persists them to a modules-load.d drop-in so they load on every future
// boot without depending on this tool running again first.
type KernelModuleResource struct {
	Env         *sysutil.Env
	Modules     []string
	PersistFile string // e.g. /etc/modules-load.d/99-node-configurator.conf
	Runner      sysutil.Runner
}

func (r *KernelModuleResource) Name() string { return "kernel-modules" }

func (r *KernelModuleResource) Check(ctx context.Context) (Result, error) {
	if len(r.Modules) == 0 {
		return Result{Name: r.Name(), Status: StatusSkipped, Detail: "no modules configured"}, nil
	}

	loaded, err := r.loadedModules()
	if err != nil {
		return Result{Name: r.Name(), Status: StatusFailed, Detail: err.Error()}, err
	}

	var missing []string
	for _, m := range r.wantModules() {
		if !loaded[m] {
			missing = append(missing, m)
		}
	}

	persistDrift := r.persistNeeded()

	if len(missing) == 0 && !persistDrift {
		return Result{Name: r.Name(), Status: StatusUnchanged}, nil
	}

	detail := ""
	if len(missing) > 0 {
		detail = fmt.Sprintf("not loaded: %s", strings.Join(missing, ", "))
	}
	if persistDrift {
		detail = joinDetails(detail, "drop-in file out of date")
	}
	return Result{Name: r.Name(), Status: StatusWouldChange, Detail: detail}, nil
}

func (r *KernelModuleResource) Apply(ctx context.Context) error {
	loaded, err := r.loadedModules()
	if err != nil {
		return err
	}

	for _, m := range r.wantModules() {
		if loaded[m] {
			continue
		}
		if _, err := r.Runner.Run(ctx, "modprobe", m); err != nil {
			return fmt.Errorf("modprobe %s: %w", m, err)
		}
	}

	if r.PersistFile != "" {
		content := renderModulesFile(r.wantModules())
		if _, err := sysutil.WriteFileIfChanged(r.PersistFile, []byte(content), 0o644); err != nil {
			return fmt.Errorf("persisting modules-load.d drop-in %s: %w", r.PersistFile, err)
		}
	}
	return nil
}

func (r *KernelModuleResource) wantModules() []string {
	mods := append([]string(nil), r.Modules...)
	sort.Strings(mods)
	return mods
}

func (r *KernelModuleResource) persistNeeded() bool {
	if r.PersistFile == "" {
		return false
	}
	existing, err := sysutil.ReadFileString(r.PersistFile)
	if err != nil {
		return true
	}
	return existing != renderModulesFile(r.wantModules())
}

// loadedModules parses /proc/modules for the set of currently loaded module
// names.
func (r *KernelModuleResource) loadedModules() (map[string]bool, error) {
	f, err := os.Open(r.Env.Path("proc", "modules"))
	if err != nil {
		return nil, fmt.Errorf("reading /proc/modules: %w", err)
	}
	defer func() { _ = f.Close() }()

	loaded := make(map[string]bool)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) > 0 {
			loaded[fields[0]] = true
		}
	}
	return loaded, scanner.Err()
}

func renderModulesFile(modules []string) string {
	var b strings.Builder
	b.WriteString("# Managed by node-configurator. Local edits will be overwritten.\n")
	for _, m := range modules {
		b.WriteString(m)
		b.WriteString("\n")
	}
	return b.String()
}
