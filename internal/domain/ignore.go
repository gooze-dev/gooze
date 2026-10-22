package domain

import (
	"go/ast"
	"go/token"
	"strings"
	"unicode"

	"gooze.dev/pkg/gooze/pkg/directive"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
)

// ignoreRule is what one scope's directives ignore: everything, or per
// lower-case mutagen name either every mutant (a nil set) or the mutants that
// write one of the listed replacements.
type ignoreRule struct {
	all   bool
	names map[string]map[string]struct{}
}

// ignores reports whether the rule ignores every mutant of mutationType.
func (r ignoreRule) ignores(mutationType mgen.Type) bool {
	if r.all {
		return true
	}

	replacements, ok := r.names[strings.ToLower(mutationType.Name)]

	return ok && replacements == nil
}

// ignoresMutant reports whether the rule ignores the mutant of mutationType
// that writes replacement; replacement is only computed when the rule lists
// replacements for mutationType.
func (r ignoreRule) ignoresMutant(mutationType mgen.Type, replacement func() string) bool {
	if r.ignores(mutationType) {
		return true
	}

	replacements := r.names[strings.ToLower(mutationType.Name)]
	if replacements == nil {
		return false
	}

	_, ok := replacements[replacement()]

	return ok
}

func mergeIgnoreRule(dst *ignoreRule, src ignoreRule) {
	if src.all {
		dst.all = true
		dst.names = nil

		return
	}

	if dst.all || len(src.names) == 0 {
		return
	}

	if dst.names == nil {
		dst.names = make(map[string]map[string]struct{}, len(src.names))
	}

	for name, replacements := range src.names {
		mergeReplacements(dst.names, name, replacements)
	}
}

// mergeReplacements adds to names that name ignores replacements; a nil set,
// every mutant, wins over any list.
func mergeReplacements(names map[string]map[string]struct{}, name string, replacements map[string]struct{}) {
	current, listed := names[name]

	switch {
	case listed && current == nil:
	case replacements == nil:
		names[name] = nil
	default:
		if current == nil {
			current = map[string]struct{}{}
			names[name] = current
		}

		for replacement := range replacements {
			current[replacement] = struct{}{}
		}
	}
}

func parseIgnoreDirective(commentText string) (ignoreRule, bool) {
	d, ok := directive.Parse(commentText)
	if !ok {
		return ignoreRule{}, false
	}

	if d.IgnoresAll() {
		return ignoreRule{all: true}, true
	}

	rule := ignoreRule{names: make(map[string]map[string]struct{}, len(d.Names))}
	for _, name := range d.Names {
		var replacements map[string]struct{}
		if name.Replacements != nil {
			replacements = map[string]struct{}{}
			for _, replacement := range name.Replacements {
				replacements[replacement] = struct{}{}
			}
		}

		mergeReplacements(rule.names, strings.ToLower(name.Text), replacements)
	}

	return rule, true
}

type ignoreIndex struct {
	file      ignoreRule
	funcByPos map[token.Pos]ignoreRule
	// funcs are the lines of the functions with a rule, for the rules that
	// ignore single mutants (see keepChanged).
	funcs []funcLines
	line  map[int]ignoreRule
}

func buildIgnoreIndex(file *ast.File, fset *token.FileSet, content []byte) ignoreIndex {
	funcByPos, funcDocGroups := buildFuncIgnoreRules(file)
	fileRule := buildFileIgnoreRule(file)
	lineRules := buildLineIgnoreRules(file, fset, content, funcDocGroups)

	return ignoreIndex{file: fileRule, funcByPos: funcByPos, funcs: funcLineRules(file, fset, funcByPos), line: lineRules}
}

func buildFuncIgnoreRules(file *ast.File) (map[token.Pos]ignoreRule, map[*ast.CommentGroup]struct{}) {
	funcByPos := make(map[token.Pos]ignoreRule)
	funcDocGroups := map[*ast.CommentGroup]struct{}{}

	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Doc == nil {
			continue
		}

		funcDocGroups[fd.Doc] = struct{}{}

		var rule ignoreRule

		for _, c := range fd.Doc.List {
			r, ok := parseIgnoreDirective(c.Text)
			if !ok {
				continue
			}

			mergeIgnoreRule(&rule, r)
		}

		if rule.all || len(rule.names) > 0 {
			funcByPos[fd.Pos()] = rule
		}
	}

	return funcByPos, funcDocGroups
}

func buildFileIgnoreRule(file *ast.File) ignoreRule {
	var rule ignoreRule

	for _, group := range file.Comments {
		if group.End() >= file.Package {
			continue
		}

		for _, c := range group.List {
			r, ok := parseIgnoreDirective(c.Text)
			if !ok {
				continue
			}

			mergeIgnoreRule(&rule, r)
		}
	}

	return rule
}

func buildLineIgnoreRules(
	file *ast.File,
	fset *token.FileSet,
	content []byte,
	funcDocGroups map[*ast.CommentGroup]struct{},
) map[int]ignoreRule {
	lineRules := make(map[int]ignoreRule)
	lineStarts := computeLineStarts(content)

	for _, group := range file.Comments {
		if group.End() < file.Package {
			continue
		}

		if _, ok := funcDocGroups[group]; ok {
			continue
		}

		for _, c := range group.List {
			r, ok := parseIgnoreDirective(c.Text)
			if !ok {
				continue
			}

			pos := fset.PositionFor(c.Slash, true)
			if pos.Line <= 0 {
				continue
			}

			targetLine := pos.Line
			if isLeadingComment(pos.Line, pos.Offset, lineStarts, content) {
				targetLine = fset.PositionFor(group.End(), true).Line + 1
			}

			current := lineRules[targetLine]
			mergeIgnoreRule(&current, r)
			lineRules[targetLine] = current
		}
	}

	return lineRules
}

func computeLineStarts(content []byte) []int {
	starts := []int{0}

	for i, b := range content {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}

	return starts
}

func isLeadingComment(line int, slashOffset int, lineStarts []int, content []byte) bool {
	if line <= 0 || line > len(lineStarts) {
		return false
	}

	start := lineStarts[line-1]
	if slashOffset < start || slashOffset > len(content) {
		return false
	}

	for _, b := range content[start:slashOffset] {
		if !unicode.IsSpace(rune(b)) {
			return false
		}
	}

	return true
}

// funcLines is the rule of a function and its first and last line.
type funcLines struct {
	first, last int
	rule        ignoreRule
}

// funcLineRules returns the lines of each function with a rule, counted like
// the line directives (honoring //line comments).
func funcLineRules(file *ast.File, fset *token.FileSet, funcByPos map[token.Pos]ignoreRule) []funcLines {
	var funcs []funcLines

	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		if rule, ok := funcByPos[fd.Pos()]; ok {
			funcs = append(funcs, funcLines{
				first: fset.PositionFor(fd.Pos(), true).Line,
				last:  fset.PositionFor(fd.End(), true).Line,
				rule:  rule,
			})
		}
	}

	return funcs
}

// funcRule returns the rule of the function line belongs to, if any.
func (ix ignoreIndex) funcRule(line int) ignoreRule {
	for _, f := range ix.funcs {
		if f.first <= line && line <= f.last {
			return f.rule
		}
	}

	return ignoreRule{}
}
