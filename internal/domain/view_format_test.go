package domain_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gooze.dev/pkg/gooze/internal/adapter"
	domain "gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
)

// viewFormatted runs View with args, expecting the text summary only when
// wantSummary, and returns what the format wrote.
func viewFormatted(t *testing.T, args domain.ViewArgs, wantSummary bool) (string, error) {
	t.Helper()

	var out bytes.Buffer

	args.Output = &out

	reporter := domainmocks.NewMockReporter(t)
	if wantSummary {
		reporter.EXPECT().DisplayReportSummary(mock.Anything, mock.Anything).Once()
	}

	wf := domain.NewWorkflow(nil, adapter.NewReportStore(), reporter, nil, nil)
	err := wf.View(context.Background(), args)

	return out.String(), err
}

func TestWorkflow_View_GitHubFormatAnnotatesSurvivors(t *testing.T) {
	out, err := viewFormatted(t, domain.ViewArgs{
		Reports:    m.Path(viewFixture(t)),
		Format:     domain.FormatGitHub,
		PathPrefix: "sub/",
	}, true)
	require.NoError(t, err)

	assert.Equal(t,
		"::notice file=sub/cmd/b.go,title=Uncovered mutant (boolean)::No test covers this boolean mutation\n"+
			"::warning file=sub/pkg/a.go,line=10,title=Surviving mutant (comparison)::Tests still pass with `if n < lo {` changed to `if n <= lo {`\n",
		out)
}

func TestWorkflow_View_GitHubFormatKeepsThresholds(t *testing.T) {
	out, err := viewFormatted(t, domain.ViewArgs{
		Reports:    m.Path(viewFixture(t)),
		Format:     domain.FormatGitHub,
		Thresholds: domain.Thresholds{Key: "threshold", Total: 100},
	}, true)

	require.Error(t, err)
	assert.Equal(t, m.ExitBelowThreshold, exitCodeOf(err))
	assert.Contains(t, out, "::warning file=pkg/a.go,line=10,")
}

func TestWorkflow_View_GitHubFormatWithoutSurvivorsPrintsNothing(t *testing.T) {
	out, err := viewFormatted(t, domain.ViewArgs{Reports: m.Path(t.TempDir()), Format: domain.FormatGitHub}, true)
	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestWorkflow_View_TextFormatWritesNothingExtra(t *testing.T) {
	out, err := viewFormatted(t, domain.ViewArgs{Reports: m.Path(viewFixture(t))}, true)
	require.NoError(t, err)
	assert.Empty(t, out)
}
