// Command node-configurator idempotently tunes Linux kernel, network, and
// performance settings for an AWS EC2 instance running Kubernetes with
// Cilium (kube-proxy-free, eBPF mode). It can be run manually or via the
// bundled systemd unit at boot, before containerd/kubelet/Cilium start.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/therealgambo/node-configurator/internal/config"
	"github.com/therealgambo/node-configurator/internal/engine"
	"github.com/therealgambo/node-configurator/internal/facts"
	"github.com/therealgambo/node-configurator/internal/profile"
	"github.com/therealgambo/node-configurator/internal/sysutil"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

const defaultConfigPath = "/etc/node-configurator/config.yaml"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 2
	}

	switch args[0] {
	case "apply":
		return runEngine(args[1:], engine.ModeApply)
	case "check":
		return runEngine(args[1:], engine.ModeCheck)
	case "facts":
		return runFacts(args[1:])
	case "profile":
		return runProfile(args[1:])
	case "version":
		fmt.Println(version)
		return 0
	case "-h", "--help", "help":
		printUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "node-configurator: unknown command %q\n\n", args[0])
		printUsage()
		return 2
	}
}

func printUsage() {
	fmt.Fprint(os.Stderr, `node-configurator: idempotent EC2 Kubernetes node tuning

Usage:
  node-configurator apply   [flags]   converge the host to desired state
  node-configurator check   [flags]   report drift without changing anything (exit 2 if changes are pending)
  node-configurator facts   [flags]   print gathered instance/host facts as JSON
  node-configurator profile [flags]   print the resolved desired-state profile as JSON
  node-configurator version           print the build version

Flags (apply/check/facts/profile):
  -config string        path to YAML config (default "`+defaultConfigPath+`")
  -log-format string     "text" or "json" (default "text"; apply/check only)
  -refresh-facts          bypass the facts cache and re-query the EC2 API
  -only string            comma-separated resource names to run exclusively (apply/check only)
  -skip string            comma-separated resource names to skip (apply/check only)
`)
}

type commonFlags struct {
	configPath   string
	logFormat    string
	refreshFacts bool
	only         string
	skip         string
}

func parseCommonFlags(name string, args []string) (*commonFlags, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	cf := &commonFlags{}
	fs.StringVar(&cf.configPath, "config", defaultConfigPath, "path to YAML config")
	fs.StringVar(&cf.logFormat, "log-format", "", `"text" or "json" (overrides config)`)
	fs.BoolVar(&cf.refreshFacts, "refresh-facts", false, "bypass the facts cache and re-query the EC2 API")
	fs.StringVar(&cf.only, "only", "", "comma-separated resource names to run exclusively")
	fs.StringVar(&cf.skip, "skip", "", "comma-separated resource names to skip")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return cf, nil
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for part := range strings.SplitSeq(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// gatherAllAndBuild loads config and facts, the two inputs every subcommand
// needs before it can do anything else.
func gatherAllAndBuild(ctx context.Context, env *sysutil.Env, cf *commonFlags) (facts.Facts, config.Config, error) {
	cfg, err := config.Load(cf.configPath)
	if err != nil {
		return facts.Facts{}, cfg, fmt.Errorf("loading config: %w", err)
	}

	g := &facts.Gatherer{Env: env, CacheTTL: time.Duration(cfg.Facts.CacheTTL), Refresh: cf.refreshFacts}
	f, err := g.Gather(ctx)
	if err != nil {
		return f, cfg, fmt.Errorf("gathering facts: %w", err)
	}
	return f, cfg, nil
}

func resolveLogFormat(cf *commonFlags, cfg config.Config) string {
	if cf.logFormat != "" {
		return cf.logFormat
	}
	if cfg.LogFormat != "" {
		return cfg.LogFormat
	}
	return "text"
}

func runEngine(args []string, mode engine.Mode) int {
	cf, err := parseCommonFlags("node-configurator", args)
	if err != nil {
		return 2
	}

	ctx := context.Background()
	env := sysutil.New()

	f, cfg, err := gatherAllAndBuild(ctx, env, cf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "node-configurator: %v\n", err)
		return 1
	}
	for _, w := range f.Warnings {
		fmt.Fprintf(os.Stderr, "node-configurator: warning: %s\n", w)
	}

	format := resolveLogFormat(cf, cfg)

	// Print what was discovered before anything is touched, so there's a
	// record of the pre-change state even if a later resource fails
	// partway through converging. JSON mode instead folds facts into the
	// single combined document written at the end, so machine consumers
	// still get exactly one parseable value out of stdout.
	if format != "json" {
		if err := engine.WriteFactsSummary(os.Stdout, f); err != nil {
			fmt.Fprintf(os.Stderr, "node-configurator: %v\n", err)
			return 1
		}
	}

	// cfg.DryRun downgrades an `apply` run to check-mode behavior; `check`
	// itself is already always read-only regardless of this setting.
	if cfg.DryRun && mode == engine.ModeApply {
		mode = engine.ModeCheck
	}

	p := profile.Build(f, cfg.ToProfileOverrides())
	resources := engine.Build(env, sysutil.ExecRunner{}, f, p, cfg.Resources.Disable, engine.Options{
		Only: splitCSV(cf.only),
		Skip: splitCSV(cf.skip),
	})

	results := engine.Run(ctx, resources, mode)

	if format == "json" {
		if err := engine.WriteJSONReport(os.Stdout, f, results); err != nil {
			fmt.Fprintf(os.Stderr, "node-configurator: %v\n", err)
			return 1
		}
	} else if err := engine.WriteTextReport(os.Stdout, results); err != nil {
		fmt.Fprintf(os.Stderr, "node-configurator: %v\n", err)
		return 1
	}

	return engine.ExitCode(results, mode)
}

func runFacts(args []string) int {
	cf, err := parseCommonFlags("node-configurator facts", args)
	if err != nil {
		return 2
	}

	f, _, err := gatherAllAndBuild(context.Background(), sysutil.New(), cf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "node-configurator: %v\n", err)
		return 1
	}
	return encodeJSON(f)
}

func runProfile(args []string) int {
	cf, err := parseCommonFlags("node-configurator profile", args)
	if err != nil {
		return 2
	}

	f, cfg, err := gatherAllAndBuild(context.Background(), sysutil.New(), cf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "node-configurator: %v\n", err)
		return 1
	}
	return encodeJSON(profile.Build(f, cfg.ToProfileOverrides()))
}

func encodeJSON(v any) int {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "node-configurator: %v\n", err)
		return 1
	}
	return 0
}
