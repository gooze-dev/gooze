//go:build linux

package adapter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestKillGroup_KillsTheLeaderAndItsChildren(t *testing.T) {
	cmd, child, _ := sleepingGroup(t)

	require.NoError(t, killGroup(cmd))
	require.Error(t, cmd.Wait())
	require.Eventually(t, func() bool { return !alive(child) }, 5*time.Second, 10*time.Millisecond)
}
