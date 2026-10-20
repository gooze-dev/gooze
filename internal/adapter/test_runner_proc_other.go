//go:build !unix

package adapter

import "os/exec"

func setProcessGroup(_ *exec.Cmd) {}
