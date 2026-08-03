package resource

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestCpumaskAll(t *testing.T) {
	cases := map[int]string{
		1:  "00000001",
		4:  "0000000f",
		32: "ffffffff",
		33: "00000001,ffffffff",
		40: "000000ff,ffffffff",
	}
	for n, want := range cases {
		if got := cpumaskAll(n); got != want {
			t.Errorf("cpumaskAll(%d) = %q, want %q", n, got, want)
		}
	}
}

const ethtoolRingOutput = `Ring parameters for eth0:
Pre-set maximums:
RX:		8192
RX Mini:	0
RX Jumbo:	0
TX:		8192
Current hardware settings:
RX:		1024
RX Mini:	0
RX Jumbo:	0
TX:		1024
`

func TestParseEthtoolRingParams(t *testing.T) {
	rp := parseEthtoolRingParams(ethtoolRingOutput)
	if rp.maxRX != 8192 || rp.maxTX != 8192 || rp.curRX != 1024 || rp.curTX != 1024 {
		t.Fatalf("unexpected ring params: %+v", rp)
	}
}

func TestNICResourceRingBufferDriftAndApply(t *testing.T) {
	env := newTestEnv(t)
	runner := &fakeRunner{outputs: map[string]string{"ethtool": ethtoolRingOutput}}

	r := &NICResource{Env: env, Runner: runner, Interface: "eth0", NumCPUs: 1, RingBufferMax: true}

	res, err := r.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusWouldChange {
		t.Fatalf("expected would_change, got %s (%s)", res.Status, res.Detail)
	}

	if err := r.Apply(context.Background()); err != nil {
		t.Fatalf("apply: %v", err)
	}

	found := false
	for _, call := range runner.calls {
		if len(call) >= 2 && call[0] == "ethtool" && call[1] == "-G" {
			found = true
			joined := strings.Join(call, " ")
			if !strings.Contains(joined, "rx 8192") || !strings.Contains(joined, "tx 8192") {
				t.Errorf("unexpected ethtool -G args: %v", call)
			}
		}
	}
	if !found {
		t.Fatalf("expected an ethtool -G call, got %v", runner.calls)
	}
}

func TestNICResourceRPSAppliesMaskToQueues(t *testing.T) {
	env := newTestEnv(t)
	rxDir := env.Path("sys", "class", "net", "eth0", "queues", "rx-0")
	writeFile(t, rxDir+"/rps_cpus", "0")

	runner := &fakeRunner{}
	r := &NICResource{Env: env, Runner: runner, Interface: "eth0", NumCPUs: 4, RPS: true}

	res, err := r.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusWouldChange {
		t.Fatalf("expected would_change, got %s (%s)", res.Status, res.Detail)
	}

	if err := r.Apply(context.Background()); err != nil {
		t.Fatalf("apply: %v", err)
	}

	data, err := os.ReadFile(rxDir + "/rps_cpus")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != cpumaskAll(4) {
		t.Fatalf("expected rps_cpus=%s, got %s", cpumaskAll(4), data)
	}
}

// TestNICResourceApplyToleratesUnsupportedRingQuery guards against a real
// bug: a NIC driver that doesn't support ring-buffer queries (or a host
// with no ethtool binary at all -- common on a virtualized CI runner's
// virtio/hv_netvsc NIC) must not fail the whole resource when Apply runs
// because RPS/XPS had real drift to fix.
func TestNICResourceApplyToleratesUnsupportedRingQuery(t *testing.T) {
	env := newTestEnv(t)
	rxDir := env.Path("sys", "class", "net", "eth0", "queues", "rx-0")
	writeFile(t, rxDir+"/rps_cpus", "0")

	runner := &fakeRunner{errs: map[string]error{"ethtool": errors.New("Operation not supported")}}
	r := &NICResource{Env: env, Runner: runner, Interface: "eth0", NumCPUs: 4, RingBufferMax: true, RPS: true}

	res, err := r.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusWouldChange {
		t.Fatalf("expected would_change from RPS drift, got %s (%s)", res.Status, res.Detail)
	}

	if err := r.Apply(context.Background()); err != nil {
		t.Fatalf("expected Apply to tolerate an unsupported ethtool ring query, got error: %v", err)
	}

	data, err := os.ReadFile(rxDir + "/rps_cpus")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != cpumaskAll(4) {
		t.Fatalf("expected RPS to still be applied despite ethtool failing, got %s", data)
	}
}

func TestNICResourceSkipsRPSOnSingleCPU(t *testing.T) {
	env := newTestEnv(t)
	r := &NICResource{Env: env, Runner: &fakeRunner{}, Interface: "eth0", NumCPUs: 1, RPS: true}
	res, err := r.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusUnchanged {
		t.Fatalf("expected unchanged (RPS skipped) on single CPU, got %s (%s)", res.Status, res.Detail)
	}
}
