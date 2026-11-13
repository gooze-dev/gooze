package adapter

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWatchLimit_KillsOnceWhenOver(t *testing.T) {
	var kills, checks atomic.Int32

	stop := watchLimit(func() { kills.Add(1) }, func() bool { return checks.Add(1) == 3 })

	require.Eventually(t, func() bool { return kills.Load() == 1 }, 5*time.Second, 10*time.Millisecond)
	time.Sleep(3 * limitCheckInterval)

	require.True(t, stop())
	require.EqualValues(t, 1, kills.Load())
	require.EqualValues(t, 3, checks.Load(), "nothing is measured after the kill")
}

func TestWatchLimit_StopsWithoutKilling(t *testing.T) {
	var kills, checks atomic.Int32

	stop := watchLimit(func() { kills.Add(1) }, func() bool {
		checks.Add(1)
		return false
	})

	require.Eventually(t, func() bool { return checks.Load() >= 2 }, 5*time.Second, 10*time.Millisecond)
	require.False(t, stop())

	measured := checks.Load()
	time.Sleep(3 * limitCheckInterval)

	require.Zero(t, kills.Load())
	require.Equal(t, measured, checks.Load(), "nothing is measured after the stop")
}

func tenBytes(t *testing.T) []string {
	t.Helper()

	one, two := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(one, "a"), []byte("123"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(one, "sub", "deep"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(one, "sub", "deep", "b"), []byte("45"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(two, "c"), []byte("67890"), 0o600))

	return []string{one, "", filepath.Join(one, "missing"), two}
}

func TestDirsSize(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("in the working directory", []byte("not counted"), 0o600))

	require.EqualValues(t, 10, dirsSize(tenBytes(t)))
	require.Zero(t, dirsSize(nil))
	require.Zero(t, dirsSize([]string{""}))
}

func TestWatchDisk_KillsOverTheLimit(t *testing.T) {
	for name, limit := range map[string]int64{"one byte under what they hold": 9, "one byte": 1} {
		t.Run(name, func(t *testing.T) {
			var kills atomic.Int32

			stop := watchDisk(func() { kills.Add(1) }, tenBytes(t), limit)

			require.Eventually(t, func() bool { return kills.Load() == 1 }, 5*time.Second, 10*time.Millisecond)
			require.True(t, stop())
		})
	}
}

func TestWatchDisk_LeavesTestsAtOrUnderTheLimit(t *testing.T) {
	for name, limit := range map[string]int64{"what they hold": 10, "no limit": 0, "negative": -1} {
		t.Run(name, func(t *testing.T) {
			var kills atomic.Int32

			stop := watchDisk(func() { kills.Add(1) }, tenBytes(t), limit)

			time.Sleep(4 * limitCheckInterval)

			require.False(t, stop())
			require.Zero(t, kills.Load())
		})
	}
}
