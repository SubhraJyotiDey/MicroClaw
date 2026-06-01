//go:build !linux
// +build !linux

package agents

import (
	"os/exec"
)

func prepareCmdAttrs(cmd *exec.Cmd) {
	// No-op for non-linux systems (Windows, macOS)
}
