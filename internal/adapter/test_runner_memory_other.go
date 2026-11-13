//go:build !linux

package adapter

import "os/exec"

// watchMemory watches nothing: the memory of a process group is measured on
// linux only.
func watchMemory(_ *exec.Cmd, _ int64) func() bool {
	return func() bool { return false }
}
