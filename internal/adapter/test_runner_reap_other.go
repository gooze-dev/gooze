//go:build !linux

package adapter

import "os/exec"

func adoptOrphans() {}

func reapOrphans(_ *exec.Cmd) {}
