package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func TestIgnoredMutant_WithoutADiffHasNoLines(t *testing.T) {
	mutation := m.Mutation{
		Source:  rpt.Source{Origin: &rpt.File{ShortPath: "a.go"}},
		Type:    mgen.Arithmetic,
		Line:    3,
		Ignored: &m.Ignored{Reason: "deferred"},
	}

	assert.Equal(t, IgnoredMutant{Path: "a.go", Line: 3, Mutagen: "arithmetic", Reason: "deferred"}, ignoredMutant(mutation))
}

func TestDirectiveReasons_AnEmptyReasonIsNoReason(t *testing.T) {
	rule, ok := parseIgnoreDirective("//gooze:ignore arithmetic reason=")

	assert.True(t, ok)
	assert.Empty(t, rule.reasons)
}
