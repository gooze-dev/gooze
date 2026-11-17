package domain

import (
	"cmp"
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"maps"
	"slices"
	"strings"

	m "gooze.dev/pkg/gooze/internal/model"
	"gooze.dev/pkg/gooze/pkg/directive"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// directiveReasons returns the reason of d for each mutagen it names, or
// under "" when it ignores them all; nil without a reason.
func directiveReasons(d directive.Directive) map[string]string {
	if d.Reason == nil || d.Reason.Text == "" {
		return nil
	}

	if d.IgnoresAll() {
		return map[string]string{"": d.Reason.Text}
	}

	reasons := make(map[string]string, len(d.Names))
	for _, name := range d.Names {
		reasons[strings.ToLower(name.Text)] = d.Reason.Text
	}

	return reasons
}

// mergeReasons adds reasons to dst; the first reason of a mutagen stays.
func mergeReasons(dst *ignoreRule, reasons map[string]string) {
	for name, reason := range reasons {
		if _, ok := dst.reasons[name]; ok {
			continue
		}

		if dst.reasons == nil {
			dst.reasons = map[string]string{}
		}

		dst.reasons[name] = reason
	}
}

// reason returns why the rule ignores mutationType: the reason of a
// directive that names it, or else of one that ignores every mutagen.
func (r ignoreRule) reason(mutationType mgen.Type) string {
	if reason, ok := r.reasons[strings.ToLower(mutationType.Name)]; ok {
		return reason
	}

	return r.reasons[""]
}

// ruleFor returns the rule that ignores a mutant of the node on nodeLine that
// changes changedLine: the node's line, else the nearest of the changed
// line, its function and the file (see visit and keepChanged).
func (ix ignoreIndex) ruleFor(nodeLine, changedLine int, mutationType mgen.Type, replacement func() string) (ignoreRule, bool) {
	if rule := ix.line[nodeLine]; rule.ignores(mutationType) {
		return rule, true
	}

	for _, rule := range []ignoreRule{ix.line[changedLine], ix.funcRule(changedLine), ix.file} {
		if rule.ignoresMutant(mutationType, replacement) {
			return rule, true
		}
	}

	return ignoreRule{}, false
}

// walkIgnored is walkMutations for the mutations a directive ignores: it
// hands fn those, with why, and none of the others.
func walkIgnored(
	mutationType mgen.Type,
	file *ast.File,
	fset *token.FileSet,
	content []byte,
	source rpt.Source,
	ignore ignoreIndex,
	funcs funcIndex,
	checks *buildChecks,
	differ Differ,
	fn func(m.Mutation) error,
) error {
	tokenFile := fset.File(file.Pos())
	if tokenFile == nil {
		return nil
	}

	var walkErr error

	constraints := map[ast.Node]bool{}
	markEquivalentSubtrees(file, constraints)
	partlyEquivalent := markPartlyEquivalent(file, constraints)

	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil || walkErr != nil {
			// Equivalent: Inspect ignores the result for a nil node, and after
			// an error every node returns here.
			return false //gooze:ignore boolean
		}

		if markTypeParams(n, constraints); constraints[n] {
			return false
		}

		nodeLine := fset.PositionFor(n.Pos(), true).Line
		mutations := generateMutationsForNode(mutationType, n, fset, content, source, checks.typed.comparison)

		for _, mutation := range checks.keepBuilding(mutationType, dropEquivalent(mutations, n, fset, partlyEquivalent), n, fset, content) {
			if !ignore.markIgnored(&mutation, nodeLine, fset, tokenFile, content, funcs) {
				continue
			}

			mutation.DiffCode = differ.Diff(content, mutation.MutatedCode)

			if walkErr = fn(mutation); walkErr != nil {
				break
			}
		}

		return walkErr == nil
	})

	return walkErr
}

// markIgnored reports whether a directive ignores mutation, which a node on
// nodeLine generates, and then sets its line, functions and why it is ignored. The enum
// bound mutants (see isEnumBoundMutant) are left out without a directive, so
// they don't count.
func (ix ignoreIndex) markIgnored(mutation *m.Mutation, nodeLine int, fset *token.FileSet, tokenFile *token.File, content []byte, funcs funcIndex) bool {
	offset := firstDifference(content, mutation.MutatedCode)
	if ix.isEnumBoundMutant(mutation.Type, offset, mutation.MutatedCode) {
		return false
	}

	changedLine := fset.PositionFor(tokenFile.Pos(offset), true).Line
	replacement := func() string { return replacementOf(content, mutation.MutatedCode) }

	rule, ok := ix.ruleFor(nodeLine, changedLine, mutation.Type, replacement)
	if !ok {
		return false
	}

	mutation.Line = lineForOffset(content, offset)
	mutation.Funcs = funcs.at(mutation.Line)
	mutation.Ignored = &m.Ignored{Reason: rule.reason(mutation.Type)}

	return true
}

// IgnoredArgs are the arguments for listing the mutants that directives
// ignore: the sources and mutants a run with the same arguments selects.
type IgnoredArgs struct {
	EstimateArgs

	Output io.Writer
}

// IgnoredMutant is a mutant a //gooze:ignore directive keeps from being
// tested.
type IgnoredMutant struct {
	Path     string
	Line     int
	Mutagen  string
	Reason   string // the directive's reason=, or ""
	Original string // the original line, trimmed
	Mutated  string // the mutated line, trimmed
}

// Ignored writes the mutants that directives ignore in the selected sources
// to args.Output, then how many each reason and each mutagen accounts for.
func (w *workflow) Ignored(ctx context.Context, args IgnoredArgs) error {
	w.setBuildTags(args.EstimateArgs)

	_, closeCache, err := w.prepareBuildCache(ctx, args.GoCache)
	if err != nil {
		return err
	}

	defer closeCache()

	ignored, err := w.ignoredMutants(ctx, args.EstimateArgs)
	if err != nil {
		return fmt.Errorf("list ignored mutants: %w", err)
	}

	if _, err := io.WriteString(args.Output, formatIgnored(ignored)); err != nil {
		return fmt.Errorf("write ignored mutants: %w", err)
	}

	return nil
}

// ignoredMutants returns the ignored mutants of the sources and mutagens
// args selects, sorted by path and line.
func (w *workflow) ignoredMutants(ctx context.Context, args EstimateArgs) ([]IgnoredMutant, error) {
	sel, err := w.selection(ctx, args)
	if err != nil {
		return nil, err
	}

	sources, err := w.scanSources(ctx, args)
	if err != nil {
		return nil, err
	}

	defer func() { _ = sources.Close() }()

	opts := sel.Options
	opts.Ignored = true

	var ignored []IgnoredMutant

	err = sources.Range(func(_ uint64, source rpt.Source) error {
		if !sel.InScope(source) {
			return nil
		}

		return w.streamMutations(ctx, source, opts, func(mutation m.Mutation) error {
			// A mutagen that can't take options streams the tested mutants.
			if mutation.Ignored != nil && sel.Includes(mutation) {
				ignored = append(ignored, ignoredMutant(mutation))
			}

			return nil
		}, sel.Types()...)
	})
	if err != nil {
		return nil, err
	}

	slices.SortStableFunc(ignored, func(a, b IgnoredMutant) int {
		return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line))
	})

	return ignored, nil
}

func ignoredMutant(mutation m.Mutation) IgnoredMutant {
	ignored := IgnoredMutant{
		Path:    mutation.Source.Key(),
		Line:    mutation.Line,
		Mutagen: mutation.Type.Name,
		Reason:  mutation.Ignored.Reason,
	}

	if changes := rpt.DiffChanges(mutation.DiffCode); len(changes) > 0 {
		ignored.Original, ignored.Mutated = strings.TrimSpace(changes[0].Original), strings.TrimSpace(changes[0].Mutated)
	}

	return ignored
}

// noReason names the mutants ignored without a reason in the counts.
const noReason = "no reason"

// formatIgnored lists ignored as "path:line: mutagen (reason): original ->
// mutated", then counts them by reason and by mutagen.
func formatIgnored(ignored []IgnoredMutant) string {
	if len(ignored) == 0 {
		return "No mutants are ignored.\n"
	}

	var out strings.Builder

	reasons, mutagens := map[string]int{}, map[string]int{}

	fmt.Fprintf(&out, "Ignored (%d):\n", len(ignored))

	for _, mutant := range ignored {
		why := ""
		if mutant.Reason != "" {
			why = " (" + mutant.Reason + ")"
		}

		fmt.Fprintf(&out, "%s:%d: %s%s: %s -> %s\n", mutant.Path, mutant.Line, mutant.Mutagen, why, mutant.Original, mutant.Mutated)

		reasons[cmp.Or(mutant.Reason, noReason)]++
		mutagens[mutant.Mutagen]++
	}

	fmt.Fprintf(&out, "\nBy reason: %s\nBy mutagen: %s\n", formatCounts(reasons), formatCounts(mutagens))

	return out.String()
}

// formatCounts lists counts as "name n", the highest first, ties by name.
func formatCounts(counts map[string]int) string {
	names := slices.SortedFunc(maps.Keys(counts), func(a, b string) int {
		return cmp.Or(cmp.Compare(counts[b], counts[a]), cmp.Compare(a, b))
	})

	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s %d", name, counts[name]))
	}

	return strings.Join(parts, ", ")
}
