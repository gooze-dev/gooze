package domain

import (
	"testing"

	"github.com/stretchr/testify/require"

	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
)

func TestMutationGenerators_CoverRegistry(t *testing.T) {
	require.Len(t, mutationGenerators, len(mgen.All()))

	for _, mg := range mgen.All() {
		require.Contains(t, mutationGenerators, mg.Type, mg.Name)
	}
}
