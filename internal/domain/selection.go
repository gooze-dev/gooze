package domain

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

// RunDirName is the directory, inside the reports directory, where a narrowed
// run (--diff, --run or --mutagens) writes its reports, so the main reports
// stay complete until `gooze report merge` folds them in.
const RunDirName = "run"

// Narrowed reports whether the selection leaves out any mutation.
func (s Selection) Narrowed() bool {
	return s.Diff != nil || s.Run != nil || s.Mutagens != nil
}

// Describe names what the selection keeps, e.g. "changed lines since main;
// functions matching ^Add$", or returns "" when it keeps everything.
func (s Selection) Describe() string {
	var parts []string

	if s.Diff != nil {
		parts = append(parts, "changed lines since "+s.Diff.Info().Ref)
	}

	if s.Run != nil {
		parts = append(parts, "functions matching "+s.Run.String())
	}

	if s.Mutagens != nil {
		names := make([]string, 0, len(s.Mutagens))
		for _, t := range s.Mutagens {
			names = append(names, t.Name)
		}

		parts = append(parts, "mutagens "+strings.Join(names, ", "))
	}

	return strings.Join(parts, "; ")
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
// comma-separated. Names are case-insensitive and de-duplicated. Any
// registered mutagen is accepted, including those that are off by default. No
// names returns nil, meaning DefaultMutations.
func ParseMutagens(values []string) ([]mgen.Type, error) {
	var types []mgen.Type

	for _, value := range values {
		for name := range strings.SplitSeq(value, ",") {
			if strings.TrimSpace(name) == "" {
				continue
			}

			mg, ok := mgen.Lookup(name)
			if !ok {
				return nil, WithExitCode(m.ExitUsage, fmt.Errorf("unknown mutagen %q; gooze has %s", strings.TrimSpace(name), strings.Join(mgen.Names(), ", ")))
			}

			if !slices.Contains(types, mg.Type) {
				types = append(types, mg.Type)
			}
		}
	}

	return types, nil
}

// selection resolves the run's Selection from args: the diff scope, the
// --run pattern and the mutagen names. Errors come before any scan.
func (w *workflow) selection(ctx context.Context, args EstimateArgs) (Selection, error) {
	var sel Selection

	if args.Run != "" {
		re, err := regexp.Compile(args.Run)
		if err != nil {
			return Selection{}, WithExitCode(m.ExitUsage, fmt.Errorf("invalid --run pattern %q: %w", args.Run, err))
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

// runReportsDir returns where a test run writes its reports: base (or its
// shard dir) for a full run; base/run (or run/shard_N) for a narrowed run,
// emptied first so it holds only this run's results.
func runReportsDir(args TestArgs, sel Selection) (m.Path, error) {
	if !sel.Narrowed() {
		return shardReportsDir(args.Reports, args.ShardIndex, args.TotalShardCount), nil
	}

	dir := shardReportsDir(m.Path(filepath.Join(string(args.Reports), RunDirName)), args.ShardIndex, args.TotalShardCount)
	if err := os.RemoveAll(string(dir)); err != nil {
		return "", fmt.Errorf("clear %s: %w", dir, err)
	}

	return dir, nil
}
