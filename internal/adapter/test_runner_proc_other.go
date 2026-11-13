//go:build !unix

package adapter

import "os/exec"

func setProcessGroup(_ *exec.Cmd) {}

func killGroup(cmd *exec.Cmd) {
	_ = cmd.Process.Kill()
}
