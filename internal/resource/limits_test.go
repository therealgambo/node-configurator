package resource

import (
	"strings"
	"testing"
)

func TestRenderLimitsConf(t *testing.T) {
	out := RenderLimitsConf(1048576, 0)
	if !strings.Contains(out, "* soft nofile 1048576") || !strings.Contains(out, "* hard nofile 1048576") {
		t.Fatalf("missing nofile lines: %s", out)
	}
	if !strings.Contains(out, "* soft nproc infinity") {
		t.Fatalf("expected nproc=0 to render as infinity: %s", out)
	}

	out2 := RenderLimitsConf(1048576, 65536)
	if !strings.Contains(out2, "* soft nproc 65536") {
		t.Fatalf("expected explicit nproc value: %s", out2)
	}
}

func TestRenderSystemdLimitDropIn(t *testing.T) {
	out := RenderSystemdLimitDropIn(1048576)
	if !strings.Contains(out, "[Service]") || !strings.Contains(out, "LimitNOFILE=1048576") {
		t.Fatalf("unexpected drop-in content: %s", out)
	}
}
