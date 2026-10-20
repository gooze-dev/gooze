package domain

import (
	"go/ast"
	"go/token"
	"strings"
	"unicode"

	"gooze.dev/pkg/gooze/pkg/directive"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
)

type ignoreRule struct {
	all   bool
	names map[string]struct{}
}

func (r ignoreRule) ignores(mutationType mgen.Type) bool {
	if r.all {
		return true
	}

	if len(r.names) == 0 {
		return false
	}

	_, ok := r.names[strings.ToLower(mutationType.Name)]

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
		dst.names = make(map[string]struct{}, len(src.names))
	}

	for name := range src.names {
		dst.names[name] = struct{}{}
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

	rule := ignoreRule{names: make(map[string]struct{}, len(d.Names))}
	for _, name := range d.Names {
		rule.names[strings.ToLower(name.Text)] = struct{}{}
	}

	return rule, true
}

type ignoreIndex struct {
	file      ignoreRule
	funcByPos map[token.Pos]ignoreRule
	line      map[int]ignoreRule
}

func buildIgnoreIndex(file *ast.File, fset *token.FileSet, content []byte) ignoreIndex {
	funcByPos, funcDocGroups := buildFuncIgnoreRules(file)
	fileRule := buildFileIgnoreRule(file)
	lineRules := buildLineIgnoreRules(file, fset, content, funcDocGroups)

	return ignoreIndex{file: fileRule, funcByPos: funcByPos, line: lineRules}
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
