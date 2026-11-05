package domain

import (
	"context"
	"slices"

	"gooze.dev/pkg/gooze/internal/domain/mutagens"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// typeInfo is what type information tells the mutagens about a file: the
// spans of its string expressions (see dropStringOps), of its integer and
// unordered expressions (see buildChecks) and the comparison mutagen's facts.
type typeInfo struct {
	strings, integers, unordered map[Span]bool
	comparison                   mutagens.Facts
}

// comparisonFacts returns what type information tells the comparison
// mutagen about source (see mutagens.Facts): the expressions a range gives
// keys in [0, len) over. It knows nothing when types don't ask for
// comparison mutants or the mutagen has no type information.
func (mg *mutagen) comparisonFacts(ctx context.Context, source rpt.Source, types []mgen.Type) mutagens.Facts {
	if mg.types == nil || !slices.Contains(types, mgen.Comparison) {
		return mutagens.Facts{}
	}

	indexed := map[[2]int]bool{}
	for span := range mg.types.IndexedExprs(ctx, source.Origin.FullPath) {
		indexed[[2]int{span.Start, span.End}] = true
	}

	return mutagens.Facts{Indexed: indexed}
}
