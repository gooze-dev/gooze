package adapter

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGroupRSS(t *testing.T) {
	proc := t.TempDir()
	stat := func(pid int, comm string, pgrp, rssPages int) {
		t.Helper()

		dir := filepath.Join(proc, strconv.Itoa(pid))
		require.NoError(t, os.Mkdir(dir, 0o700))

		line := strconv.Itoa(pid) + " (" + comm + ") S 1 " + strconv.Itoa(pgrp) +
			" 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 " + strconv.Itoa(rssPages) + " 25 26\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "stat"), []byte(line), 0o600))
	}

	stat(100, "go", 100, 10)
	stat(101, "a b) (c", 100, 5)
	stat(102, "x.test", 100, 0)
	stat(900, "nine", 100, 2)
	stat(200, "other", 201, 1000)
	stat(201, "100", 201, 100)

	require.NoError(t, os.Mkdir(filepath.Join(proc, "self"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(proc, "self", "stat"), []byte("100 (go) S 1 100 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 999 25\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(proc, "uptime"), []byte("1 (x) S 1 100 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 999 25\n"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(proc, "300"), 0o700))
	require.NoError(t, os.Mkdir(filepath.Join(proc, "301"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(proc, "301", "stat"), []byte("301 (short) S 1 100 6\n"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(proc, "302"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(proc, "302", "stat"), []byte("no command name\n"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(proc, "303"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(proc, "303", "stat"), []byte("303 (bad) S 1 100 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 many 25\n"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(proc, "304"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(proc, "304", "stat"), []byte("304 (bad) S 1 group 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 7 25\n"), 0o600))

	page := int64(os.Getpagesize())

	require.NoError(t, os.Mkdir(filepath.Join(proc, "305"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(proc, "305", "stat"), []byte("305 (cut) S 1 100 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23\n"), 0o600))

	require.Equal(t, 17*page, groupRSS(proc, 100))
	require.Equal(t, 1100*page, groupRSS(proc, 201))
	require.Zero(t, groupRSS(proc, 999), "no process of the group")
	require.Zero(t, groupRSS(filepath.Join(proc, "missing"), 100), "no proc directory")
}

func sleepingGroup(t *testing.T) (*exec.Cmd, int, string) {
	t.Helper()

	cmd := exec.Command("sh", "-c", "sleep 30 & echo $!; wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
	})

	line, err := bufio.NewReader(stdout).ReadString('\n')
	require.NoError(t, err)

	child, err := strconv.Atoi(strings.TrimSpace(line))
	require.NoError(t, err)

	proc := t.TempDir()
	pid := strconv.Itoa(cmd.Process.Pid)
	require.NoError(t, os.Mkdir(filepath.Join(proc, pid), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(proc, pid, "stat"),
		[]byte(pid+" (sh) S 1 "+pid+" 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 10 25\n"), 0o600))

	return cmd, child, proc
}

func alive(pid int) bool {
	stat, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return false
	}

	state := strings.Fields(string(stat[strings.LastIndexByte(string(stat), ')')+1:]))

	return len(state) > 0 && state[0] != "Z"
}

func TestWatchGroupMemory_KillsTheGroupOverTheLimit(t *testing.T) {
	page := int64(os.Getpagesize())

	for name, limit := range map[string]int64{"one byte under what it holds": 10*page - 1, "one byte": 1} {
		t.Run(name, func(t *testing.T) {
			cmd, child, proc := sleepingGroup(t)

			stop := watchGroupMemory(proc, cmd.Process.Pid, limit)

			require.Error(t, cmd.Wait(), "the shell was killed")
			require.True(t, stop())
			require.Eventually(t, func() bool { return !alive(child) }, 5*time.Second, 10*time.Millisecond, "its child too")
		})
	}
}

func TestWatchGroupMemory_LeavesTheGroupAtOrUnderTheLimit(t *testing.T) {
	page := int64(os.Getpagesize())

	for name, limit := range map[string]int64{"what it holds": 10 * page, "no limit": 0, "negative": -1} {
		t.Run(name, func(t *testing.T) {
			cmd, child, proc := sleepingGroup(t)

			stop := watchGroupMemory(proc, cmd.Process.Pid, limit)

			time.Sleep(4 * limitCheckInterval)

			require.False(t, stop())
			require.True(t, alive(cmd.Process.Pid))
			require.True(t, alive(child))
		})
	}
}
