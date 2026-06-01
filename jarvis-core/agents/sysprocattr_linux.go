//go:build linux
// +build linux

package agents

import (
	"os/exec"
)

func prepareCmdAttrs(cmd *exec.Cmd) {
	// No-op to avoid operation not permitted (CAP_SETUID) errors when running as non-root user (e.g. 'pi') on RPi.
	// Sandbox security is enforced via resolvePath checks and healer validateCode.
}
