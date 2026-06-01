//go:build linux
// +build linux

package agents

import (
	"os/exec"
	"syscall"
)

func prepareCmdAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: 65534, // nobody
			Gid: 65534, // nogroup
		},
	}
}
