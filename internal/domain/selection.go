package domain

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// Selection narrows which mutations a run generates and tests: the lines of a
// diff, functions matching a --run pattern, and a subset of mutagens. The
// zero value selects everything.
type Selection struct {
	// Diff limits the run to changed lines; nil means no diff mode.
	Diff *DiffScope
	// Run, like go test -run, keeps mutations in functions whose name
	// ("Name", or "Type.Name" for a method) it matches; nil keeps all.
	Run *regexp.Regexp
	// Mutagens are the mutagens to run; nil means DefaultMutations.
	Mutagens []mgen.Type
}

// Types returns the mutagens to generate.
func (s Selection) Types() []mgen.Type {
	if s.Mutagens == nil {
		return DefaultMutations
	}

	return s.Mutagens
}

// InScope reports whether any mutation of source can be selected.
func (s Selection) InScope(source rpt.Source) bool {
	return s.Diff.InScope(source)
}

// Includes reports whether mutation is selected. Mutagens are not checked
// here: Types already limits what is generated.
func (s Selection) Includes(mutation m.Mutation) bool {
	if !s.Diff.Includes(mutation.Source, mutation.Line) {
		return false
	}

	return s.Run == nil || slices.ContainsFunc(mutation.Funcs, s.Run.MatchString)
}

// ReportScope returns what reports of source record about the selection, or
// nil when the source is tested in full by every default mutagen.
func (s Selection) ReportScope(source rpt.Source) *rpt.Scope {
	scope := s.Diff.ReportScope(source)

	if s.Run == nil && s.Mutagens == nil {
		return scope
	}

	if scope == nil {
		scope = &rpt.Scope{}
	}

	if s.Run != nil {
		scope.Run = s.Run.String()
	}

	for _, t := range s.Mutagens {
		scope.Mutagens = append(scope.Mutagens, t.Name)
	}

	return scope
}

// ParseMutagens resolves mutagen names, given as a list whose items may be
// comma-separated. Names are case-insensitive and de-duplicated. Only the
// mutagens gooze runs (DefaultMutations) are accepted. No names returns nil,
// meaning all of them.
func ParseMutagens(values []string) ([]mgen.Type, error) {
	var types []mgen.Type

	for _, value := range values {
		for name := range strings.SplitSeq(value, ",") {
			name = strings.ToLower(strings.TrimSpace(name))
			if name == "" {
				continue
			}

			i := slices.IndexFunc(DefaultMutations, func(t mgen.Type) bool { return t.Name == name })
			if i < 0 {
				return nil, fmt.Errorf("unknown mutagen %q; gooze runs %s", name, defaultMutagenNames())
			}

			if !slices.Contains(types, DefaultMutations[i]) {
				types = append(types, DefaultMutations[i])
			}
		}
	}

	return types, nil
}

func defaultMutagenNames() string {
	names := make([]string, 0, len(DefaultMutations))
	for _, t := range DefaultMutations {
		names = append(names, t.Name)
	}

	return strings.Join(names, ", ")
}

// selection resolves the run's Selection from args: the diff scope, the
// --run pattern and the mutagen names. Errors come before any scan.
func (w *workflow) selection(ctx context.Context, args EstimateArgs) (Selection, error) {
	var sel Selection

	if args.Run != "" {
		re, err := regexp.Compile(args.Run)
		if err != nil {
			return Selection{}, fmt.Errorf("invalid --run pattern %q: %w", args.Run, err)
		}

		sel.Run = re
	}

	mutagens, err := ParseMutagens(args.Mutagens)
	if err != nil {
		return Selection{}, err
	}

	sel.Mutagens = mutagens

	if sel.Diff, err = w.diffScope(ctx, args); err != nil {
		return Selection{}, err
	}

	return sel, nil
}
