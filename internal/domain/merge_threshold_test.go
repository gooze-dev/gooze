package domain_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// mergeShardsWith writes two shards with one survivor each under a new run
// dir, both recording selection, and merges them with the given gates.
func mergeShardsWith(t *testing.T, selection rpt.Selection) error {
	t.Helper()

	runDir := filepath.Join(t.TempDir(), domain.RunDirName)

	for i, id := range []string{"in-a", "next"} {
		shard := filepath.Join(runDir, domain.ShardDirPrefix+string(rune('0'+i)))
		saveReports(t, shard, mergeReport(mergeSource("h1"), id, rpt.Survived, runScope))

		if selection.Diff != "" || selection.Run != "" {
			saveSelection(t, shard, selection)
		}
	}

	return mergeWorkflow(t).Merge(context.Background(), domain.MergeArgs{
		Reports:        m.Path(runDir),
		Thresholds:     domain.Thresholds{Key: "threshold", Total: 85},
		DiffThresholds: domain.Thresholds{Key: "threshold.diff", Total: 100},
	})
}

func TestMerge_DiffShardsCheckTheDiffThresholds(t *testing.T) {
	err := mergeShardsWith(t, diffSelection)
	require.EqualError(t, err, "mutation score 0.00% is below threshold.diff.total 100%")
	assert.Equal(t, m.ExitBelowThreshold, exitCodeOf(err))
}

func TestMerge_OtherShardsCheckThePlainThresholds(t *testing.T) {
	require.EqualError(t, mergeShardsWith(t, rpt.Selection{}), "mutation score 0.00% is below threshold.total 85%")
	require.EqualError(t, mergeShardsWith(t, rpt.Selection{Run: "^InRange$"}), "mutation score 0.00% is below threshold.total 85%")
}
