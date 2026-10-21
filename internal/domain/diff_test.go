package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gooze.dev/pkg/gooze/internal/domain"
)

func TestFileChange_Contains(t *testing.T) {
	t.Parallel()

	whole := domain.FileChange{Whole: true}
	assert.True(t, whole.Contains(1))
	assert.True(t, whole.Contains(1000))

	lines := domain.FileChange{Lines: []domain.LineRange{{Start: 3, End: 3}, {Start: 10, End: 12}}}
	for line, want := range map[int]bool{1: false, 2: false, 3: true, 4: false, 9: false, 10: true, 11: true, 12: true, 13: false} {
		assert.Equal(t, want, lines.Contains(line), "line %d", line)
	}

	assert.False(t, domain.FileChange{}.Contains(1), "a change with no lines contains none")
}
