package adapter

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"
)

const strayHelperEnv = "GOOZE_TEST_STRAY_HELPER"

func processState(pid int) string {
	stat, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return ""
	}

	fields := strings.Fields(string(stat[strings.LastIndexByte(string(stat), ')')+1:]))
	if len(fields) == 0 {
		return ""
	}

	return fields[0]
}

func TestAdoptOrphans(t *testing.T) {
	adoptOrphans()

	var adopts int

	_, _, errno := syscall.Syscall(syscall.SYS_PRCTL, 37, uintptr(unsafe.Pointer(&adopts)), 0)
	require.Zero(t, errno)
	require.Equal(t, 1, adopts)
}

func TestReapOrphans_KillsTheGroupAndLeavesNoZombie(t *testing.T) {
	adoptOrphans()

	cmd, child, _ := sleepingGroup(t)

	require.NoError(t, cmd.Process.Kill())
	require.Error(t, cmd.Wait())
	require.True(t, alive(child), "the orphan outlives the shell")

	done := make(chan struct{})

	go func() {
		reapOrphans(cmd)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		require.Fail(t, "reapOrphans waits for a process it didn't kill")
	}

	require.Empty(t, processState(child), "neither alive nor a zombie")
}

func TestHelperStrayProcess(t *testing.T) {
	if os.Getenv(strayHelperEnv) == "" {
		t.Skip("run by TestReapStrays_ReapsAnAdoptedProcessOfAnotherSession")
	}

	cmd := exec.Command("sleep", "0.3")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	require.NoError(t, cmd.Start())

	fmt.Println("stray", cmd.Process.Pid)
}

func TestReapStrays_ReapsAnAdoptedProcessOfAnotherSession(t *testing.T) {
	adoptOrphans()

	helper := exec.Command(os.Args[0], "-test.run=^TestHelperStrayProcess$")
	helper.Env = append(os.Environ(), strayHelperEnv+"=1")

	out, err := helper.Output()
	require.NoError(t, err)

	var stray int

	for line := range strings.SplitSeq(string(out), "\n") {
		if pid, ok := strings.CutPrefix(line, "stray "); ok {
			stray, err = strconv.Atoi(pid)
			require.NoError(t, err)
		}
	}

	require.NotZero(t, stray, string(out))
	require.Eventually(t, func() bool { return processState(stray) == "Z" }, 5*time.Second, 10*time.Millisecond, "adopted, and nobody waits for it")

	reapStrays(procDir)

	require.Empty(t, processState(stray))
}

func TestReapStrays_LeavesTheChildrenOfThisSession(t *testing.T) {
	cmd := exec.Command("true")
	require.NoError(t, cmd.Start())
	require.Eventually(t, func() bool { return processState(cmd.Process.Pid) == "Z" }, 5*time.Second, 10*time.Millisecond)

	reapStrays(procDir)

	require.NoError(t, cmd.Wait(), "its exit status is still there to read")
}
