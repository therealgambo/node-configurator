package engine

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/therealgambo/node-configurator/internal/facts"
	"github.com/therealgambo/node-configurator/internal/resource"
)

func TestWriteFactsSummaryFullyDiscovered(t *testing.T) {
	f := facts.Facts{
		InstanceID: "i-0123456789abcdef0", InstanceType: "m6i.xlarge", Family: "m6i",
		Region: "us-east-1", AvailabilityZone: "us-east-1a", Arch: "x86_64",
		VCPUs: 4, MemMiB: 16384, NetworkBandwidthGbps: 6.25, EnaSupport: true, MaxENIs: 4,
		Hypervisor: "nitro", Source: "ec2-api",
		KernelVersion: "6.1.0-aws",
	}
	f.Distro.ID = "ubuntu"
	f.Distro.VersionID = "22.04"

	var buf bytes.Buffer
	if err := WriteFactsSummary(&buf, f); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	for _, want := range []string{
		"source: ec2-api", "m6i.xlarge", "family m6i", "x86_64", "nitro hypervisor",
		"us-east-1", "us-east-1a", "4 vCPUs", "16384 MiB", "6.25 Gbps", "(ENA)", "maxENIs=4",
		"ubuntu 22.04", "6.1.0-aws",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected summary to contain %q, got:\n%s", want, out)
		}
	}
}

func TestWriteFactsSummaryLocalFallback(t *testing.T) {
	f := facts.Facts{Source: "local-fallback", Arch: "arm64", VCPUs: 2, MemMiB: 1024}

	var buf bytes.Buffer
	if err := WriteFactsSummary(&buf, f); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if !strings.Contains(out, "source: local-fallback") {
		t.Errorf("expected local-fallback source, got:\n%s", out)
	}
	if !strings.Contains(out, "unknown (arch arm64)") {
		t.Errorf("expected unknown instance placeholder for a missing InstanceType, got:\n%s", out)
	}
	if strings.Contains(out, "Region:") {
		t.Errorf("expected no Region line when Region is empty, got:\n%s", out)
	}
}

func TestWriteJSONReportIsOneParseableDocument(t *testing.T) {
	f := facts.Facts{InstanceType: "m6i.xlarge", Source: "ec2-api"}
	results := []resource.Result{{Name: "sysctl", Status: resource.StatusChanged}}

	var buf bytes.Buffer
	if err := WriteJSONReport(&buf, f, results); err != nil {
		t.Fatal(err)
	}

	var report Report
	dec := json.NewDecoder(&buf)
	if err := dec.Decode(&report); err != nil {
		t.Fatalf("expected a single decodable JSON document, got error: %v", err)
	}
	if dec.More() {
		t.Fatal("expected exactly one JSON value in the output, found more")
	}
	if report.Facts.InstanceType != "m6i.xlarge" {
		t.Errorf("unexpected facts in report: %+v", report.Facts)
	}
	if len(report.Results) != 1 || report.Results[0].Name != "sysctl" {
		t.Errorf("unexpected results in report: %+v", report.Results)
	}
}
