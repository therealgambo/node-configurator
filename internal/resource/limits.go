package resource

import "fmt"

// RenderLimitsConf renders a /etc/security/limits.d drop-in raising the
// open-file and process limits for interactive/PAM-authenticated sessions
// (e.g. an operator's SSH login). This does not affect services started
// directly by systemd — see RenderSystemdLimitDropIn for those.
func RenderLimitsConf(nofile, nproc uint64) string {
	nprocStr := "infinity"
	if nproc > 0 {
		nprocStr = fmt.Sprintf("%d", nproc)
	}
	return fmt.Sprintf(`# Managed by node-configurator. Local edits will be overwritten.
* soft nofile %d
* hard nofile %d
* soft nproc %s
* hard nproc %s
`, nofile, nofile, nprocStr, nprocStr)
}

// RenderSystemdLimitDropIn renders a systemd unit drop-in that raises
// LimitNOFILE for one service (e.g. containerd, kubelet, cilium). Services
// started directly by systemd don't go through PAM, so
// /etc/security/limits.d alone does not affect them.
func RenderSystemdLimitDropIn(nofile uint64) string {
	return fmt.Sprintf(`# Managed by node-configurator. Local edits will be overwritten.
[Service]
LimitNOFILE=%d
`, nofile)
}
