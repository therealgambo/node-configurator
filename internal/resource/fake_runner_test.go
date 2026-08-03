package resource

import "context"

// fakeRunner is a sysutil.Runner test double. outputs, keyed by command
// name, lets a test can a specific command's stdout (e.g. "ethtool" or
// "systemctl"); errs, also keyed by command name, lets a test make a
// specific command fail without affecting others. listUnitsOutput is a
// convenience for the common `systemctl list-units` case used by
// SwapResource.
type fakeRunner struct {
	calls           [][]string
	err             error
	errs            map[string]error
	outputs         map[string]string
	listUnitsOutput string
}

func (f *fakeRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	if f.err != nil {
		return "", f.err
	}
	if err, ok := f.errs[name]; ok {
		return "", err
	}
	if name == "systemctl" && len(args) > 0 && args[0] == "list-units" {
		return f.listUnitsOutput, nil
	}
	if out, ok := f.outputs[name]; ok {
		return out, nil
	}
	return "", nil
}
