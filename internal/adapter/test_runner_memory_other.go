//go:build !linux

package adapter

import "os/exec"

func watchMemory(_ *exec.Cmd, _ int64) func() bool {
	return func() bool { return false }
}
