package engine

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/therealgambo/node-configurator/internal/facts"
	"github.com/therealgambo/node-configurator/internal/profile"
	"github.com/therealgambo/node-configurator/internal/resource"
	"github.com/therealgambo/node-configurator/internal/sysutil"
)

// fakeResource is a minimal resource.Resource test double: it reports a
// fixed Check status and records whether Apply was called.
type fakeResource struct {
	name        string
	checkStatus resource.Status
	applyErr    error
	applyCalled bool
}

func (f *fakeResource) Name() string { return f.name }

func (f *fakeResource) Check(ctx context.Context) (resource.Result, error) {
	return resource.Result{Name: f.name, Status: f.checkStatus}, nil
}

func (f *fakeResource) Apply(ctx context.Context) error {
	f.applyCalled = true
	return f.applyErr
}

func TestRunModeCheckNeverApplies(t *testing.T) {
	r := &fakeResource{name: "widget", checkStatus: resource.StatusWouldChange}
	results := Run(context.Background(), []resource.Resource{r}, ModeCheck)

	if r.applyCalled {
		t.Fatal("expected Apply to never be called in ModeCheck")
	}
	if len(results) != 1 || results[0].Status != resource.StatusWouldChange {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestRunModeApplyConvergesDrift(t *testing.T) {
	r := &fakeResource{name: "widget", checkStatus: resource.StatusWouldChange}
	results := Run(context.Background(), []resource.Resource{r}, ModeApply)

	if !r.applyCalled {
		t.Fatal("expected Apply to be called in ModeApply when Check found drift")
	}
	if results[0].Status != resource.StatusChanged {
		t.Fatalf("expected status changed after successful apply, got %s", results[0].Status)
	}
}

func TestRunModeApplySkipsUnchanged(t *testing.T) {
	r := &fakeResource{name: "widget", checkStatus: resource.StatusUnchanged}
	Run(context.Background(), []resource.Resource{r}, ModeApply)
	if r.applyCalled {
		t.Fatal("expected Apply to not be called when Check reports no drift")
	}
}

func TestRunModeApplyReportsApplyFailure(t *testing.T) {
	r := &fakeResource{name: "widget", checkStatus: resource.StatusWouldChange, applyErr: errors.New("boom")}
	results := Run(context.Background(), []resource.Resource{r}, ModeApply)
	if results[0].Status != resource.StatusFailed {
		t.Fatalf("expected failed status, got %s", results[0].Status)
	}
	if results[0].Detail != "boom" {
		t.Fatalf("expected apply error surfaced in Detail, got %q", results[0].Detail)
	}
}

func TestRunContinuesAfterFailure(t *testing.T) {
	r1 := &fakeResource{name: "first", checkStatus: resource.StatusWouldChange, applyErr: errors.New("boom")}
	r2 := &fakeResource{name: "second", checkStatus: resource.StatusWouldChange}
	results := Run(context.Background(), []resource.Resource{r1, r2}, ModeApply)

	if len(results) != 2 {
		t.Fatalf("expected both resources to run despite the first failing, got %d results", len(results))
	}
	if results[1].Status != resource.StatusChanged {
		t.Fatalf("expected second resource to still converge, got %s", results[1].Status)
	}
}

func TestExitCode(t *testing.T) {
	cases := []struct {
		name    string
		results []resource.Result
		mode    Mode
		want    int
	}{
		{"all unchanged", []resource.Result{{Status: resource.StatusUnchanged}}, ModeApply, 0},
		{"apply changed", []resource.Result{{Status: resource.StatusChanged}}, ModeApply, 0},
		{"check pending", []resource.Result{{Status: resource.StatusWouldChange}}, ModeCheck, 2},
		{"apply failed", []resource.Result{{Status: resource.StatusFailed}}, ModeApply, 1},
		{"failure beats pending", []resource.Result{{Status: resource.StatusFailed}, {Status: resource.StatusWouldChange}}, ModeCheck, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExitCode(tc.results, tc.mode); got != tc.want {
				t.Errorf("ExitCode() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestBuildIncludesCoreResourcesAndRespectsFilters(t *testing.T) {
	env := &sysutil.Env{Root: t.TempDir()}
	f := facts.Facts{VCPUs: 4, MemMiB: 16384}
	p := profile.Build(f, profile.Overrides{})

	all := Build(env, sysutil.ExecRunner{}, f, p, nil, Options{})
	names := resourceNames(all)

	for _, want := range []string{"sysctl", "kernel-modules", "swap", "cgroup-v2", "bpffs-mount", "cpu-governor"} {
		if !containsName(names, want) {
			t.Errorf("expected resource %q to be built, got %v", want, names)
		}
	}

	// limits.NoFile > 0 by default, so limits-conf plus one systemd drop-in
	// per service should be present.
	if !containsName(names, "limits-conf") || !containsName(names, "systemd-limits-kubelet") {
		t.Errorf("expected limits resources to be built, got %v", names)
	}

	disabled := Build(env, sysutil.ExecRunner{}, f, p, []string{"cpu-governor"}, Options{})
	if containsName(resourceNames(disabled), "cpu-governor") {
		t.Error("expected cpu-governor to be excluded when disabled via config")
	}

	onlySysctl := Build(env, sysutil.ExecRunner{}, f, p, nil, Options{Only: []string{"sysctl"}})
	if len(onlySysctl) != 1 || onlySysctl[0].Name() != "sysctl" {
		t.Errorf("expected Only filter to restrict to a single resource, got %v", resourceNames(onlySysctl))
	}

	skipSwap := Build(env, sysutil.ExecRunner{}, f, p, nil, Options{Skip: []string{"swap"}})
	if containsName(resourceNames(skipSwap), "swap") {
		t.Error("expected swap to be excluded via Skip")
	}
}

func TestBuildOmitsLimitsWhenNoFileIsZero(t *testing.T) {
	env := &sysutil.Env{Root: t.TempDir()}
	f := facts.Facts{VCPUs: 2, MemMiB: 2048}
	nofile := uint64(0)
	p := profile.Build(f, profile.Overrides{})
	p.Limits.NoFile = nofile

	all := Build(env, sysutil.ExecRunner{}, f, p, nil, Options{})
	if containsName(resourceNames(all), "limits-conf") {
		t.Error("expected no limits resources when Limits.NoFile is 0")
	}
}

func resourceNames(resources []resource.Resource) []string {
	names := make([]string, len(resources))
	for i, r := range resources {
		names[i] = r.Name()
	}
	return names
}

func containsName(names []string, want string) bool {
	return slices.Contains(names, want)
}
