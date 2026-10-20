package domain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInShard(t *testing.T) {
	ids := []string{"hash-0", "hash-1", "hash-2", "hash-3", "hash-4", "hash-5"}

	// A non-positive total disables sharding: everything is included.
	for _, id := range ids {
		require.True(t, inShard(id, 0, 0))
		require.True(t, inShard(id, 0, -3))
	}

	// An out-of-range shard index matches nothing.
	for _, id := range ids {
		require.False(t, inShard(id, 2, 2))
		require.False(t, inShard(id, 3, 2))
	}

	// Each id belongs to exactly one shard of a valid partition.
	const total = 3
	for _, id := range ids {
		matches := 0
		for shard := range total {
			if inShard(id, shard, total) {
				matches++
			}
		}

		require.Equal(t, 1, matches, "id %q must belong to exactly one shard", id)
	}
}
