package adapter

import (
	"bytes"
	"os/exec"
	"strconv"
	"syscall"
)

const prSetChildSubreaper = 36

func adoptOrphans() {
	var unused uintptr

	syscall.Syscall(syscall.SYS_PRCTL, prSetChildSubreaper, 1, unused) //nolint:errcheck // without it, orphans go to init as before.
}

func reapOrphans(cmd *exec.Cmd) {
	_ = killGroup(cmd)

	var err error
	for err == nil {
		// Equivalent: with WNOHANG the loop spins until the same processes are reaped.
		_, err = syscall.Wait4(-cmd.Process.Pid, nil, 0, nil) //gooze:ignore numbers
	}

	reapStrays(procDir)
}

func reapStrays(procDir string) {
	var unused uintptr

	session, _, _ := syscall.RawSyscall(syscall.SYS_GETSID, 0, unused, unused)
	own := []byte(strconv.Itoa(int(session)))

	eachProcess(procDir, func(pid int, fields [][]byte) {
		if !bytes.Equal(fields[sessionField], own) {
			_, _ = syscall.Wait4(pid, nil, syscall.WNOHANG, nil)
		}
	})
}
