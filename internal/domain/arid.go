package domain

import (
	"go/ast"
	"go/token"
	gopath "path"
	"slices"
	"strconv"
	"strings"
)

// AridReason is the reason of the mutants left out because their code is
// arid: code whose mutants only a test of a log text, a timeout, a flag
// default or a metric could kill.
const AridReason = "arid"

// The families of arid code, as AridOptions.Rules names them.
const (
	AridLogging = "logging"
	AridTime    = "time"
	AridFlags   = "flags"
	AridMetrics = "metrics"
)

// AridOptions select the arid code a run doesn't mutate.
type AridOptions struct {
	// Enabled turns the rules on; without it nothing is arid.
	Enabled bool
	// Rules names the families that apply; empty applies them all.
	Rules []string
	// Calls adds calls that are arid whole, as receiver.Function: the
	// receiver is an import path, a package name or the name of a variable
	// or field, and either side may be *.
	Calls []string
}

// anyName is the pattern side that matches every name.
const anyName = "*"

// aridCalls are the calls of each family that are arid whole, arguments
// included.
var aridCalls = map[string][]string{
	AridLogging: {
		"log.*", "slog.*", "logger.*", "Logger.*", "Log.*", "logrus.*", "zap.*", "zlog.*", "klog.*", "glog.*", "logr.*",
		"t.Log", "t.Logf", "b.Log", "b.Logf", "tb.Log", "tb.Logf",
	},
	AridFlags:   {"flag.*", "pflag.*", "*.Flags", "*.PersistentFlags"},
	AridMetrics: {"prometheus.*", "promauto.*", "metrics.*", "otel.*", "span.*", "*.WithLabelValues", "*.Observe"},
}

// aridExits start the names of the calls that end the program or the
// goroutine, as log.Fatalf and log.Panicln do: a test can tell those from
// the others, so they are never arid.
var aridExits = []string{"Fatal", "Panic"}

// aridTimeArgs are the calls that take a time value, and which argument it
// is: only that argument is arid.
var aridTimeArgs = map[string]int{
	"context.WithTimeout":  1,
	"context.WithDeadline": 1,
	"time.After":           0,
	"time.AfterFunc":       0,
	"time.NewTicker":       0,
	"time.NewTimer":        0,
	"time.Sleep":           0,
	"time.Tick":            0,
	"*.SetDeadline":        0,
	"*.SetReadDeadline":    0,
	"*.SetWriteDeadline":   0,
}

// offsetRange is the code from the offset start up to end.
type offsetRange struct{ start, end int }

// aridRanges is the arid code of a file.
type aridRanges []offsetRange

// contains reports whether offset is in arid code.
func (r aridRanges) contains(offset int) bool {
	return slices.ContainsFunc(r, func(o offsetRange) bool { return o.start <= offset && offset < o.end })
}

// callPattern matches calls by their receiver and function name.
type callPattern struct {
	receiver, function string
}

// parseCallPattern splits receiver.Function at its last dot. A pattern
// without one has no receiver, which no call has, so it matches none.
func parseCallPattern(pattern string) callPattern {
	dot := strings.LastIndex(pattern, ".")

	return callPattern{receiver: pattern[:max(dot, 0)], function: pattern[dot+1:]}
}

// matches reports whether the pattern matches a call of function on one of
// the receiver's names.
func (p callPattern) matches(receivers []string, function string) bool {
	if p.function != anyName && p.function != function {
		return false
	}

	return p.receiver == anyName || slices.Contains(receivers, p.receiver)
}

// aridFinder finds the arid code of one file.
type aridFinder struct {
	file    *token.File
	imports map[string]string // import path by the name the file uses
	calls   []callPattern
	time    bool
	ranges  aridRanges
}

// findArid returns the arid code of file under opts.
func findArid(file *ast.File, tokenFile *token.File, opts AridOptions) aridRanges {
	if !opts.Enabled || tokenFile == nil {
		return nil
	}

	finder := &aridFinder{file: tokenFile, imports: importNames(file), calls: aridPatterns(opts)}
	finder.time = len(opts.Rules) == 0 || slices.Contains(opts.Rules, AridTime)

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			finder.call(node)
		case *ast.IfStmt:
			if finder.aridBranches(node) {
				finder.add(node.Cond)
			}
		}

		return true
	})

	return finder.ranges
}

// aridPatterns returns the patterns of the calls that are arid whole: those
// of the selected families and opts.Calls.
func aridPatterns(opts AridOptions) []callPattern {
	var patterns []callPattern

	for _, family := range []string{AridLogging, AridFlags, AridMetrics} {
		if len(opts.Rules) > 0 && !slices.Contains(opts.Rules, family) {
			continue
		}

		for _, call := range aridCalls[family] {
			patterns = append(patterns, parseCallPattern(call))
		}
	}

	for _, call := range opts.Calls {
		patterns = append(patterns, parseCallPattern(call))
	}

	return patterns
}

// importNames returns the import path of each package by the name file uses
// for it: its alias, or the last element of its path without a major version.
func importNames(file *ast.File) map[string]string {
	names := make(map[string]string, len(file.Imports))

	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}

		name := packageName(path)
		if spec.Name != nil {
			name = spec.Name.Name
		}

		names[name] = path
	}

	return names
}

// packageName returns the name a package of path most likely has: its last
// element, or the one before a major version such as v2.
func packageName(path string) string {
	name := gopath.Base(path)
	if isMajorVersion(name) {
		name = gopath.Base(gopath.Dir(path))
	}

	return name
}

// isMajorVersion reports whether element is a major version suffix: v and
// digits.
func isMajorVersion(element string) bool {
	digits, ok := strings.CutPrefix(element, "v")
	if !ok || digits == "" {
		return false
	}

	return strings.Trim(digits, "0123456789") == ""
}

func (f *aridFinder) add(n ast.Node) {
	f.ranges = append(f.ranges, offsetRange{start: f.file.Offset(n.Pos()), end: f.file.Offset(n.End())})
}

// call records call when it is arid whole, or its time argument when it
// takes one.
func (f *aridFinder) call(call *ast.CallExpr) {
	if f.aridCall(call) {
		f.add(call)
		return
	}

	if !f.time {
		return
	}

	receivers, function := f.callee(call)

	for pattern, arg := range aridTimeArgs {
		if parseCallPattern(pattern).matches(receivers, function) && arg < len(call.Args) {
			f.add(call.Args[arg])
		}
	}
}

// aridCall reports whether call is arid whole: it matches a pattern, or it
// is a method of the result of a call that is, as in log.With(...).Info(...).
func (f *aridFinder) aridCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	if inner, ok := sel.X.(*ast.CallExpr); ok && f.aridCall(inner) {
		return true
	}

	receivers, function := f.callee(call)

	if slices.ContainsFunc(aridExits, func(prefix string) bool { return strings.HasPrefix(function, prefix) }) {
		return false
	}

	return slices.ContainsFunc(f.calls, func(p callPattern) bool { return p.matches(receivers, function) })
}

// callee returns the function name of a call through a selector and the
// names of its receiver: an identifier's name and, for a package, its import
// path and package name; a field's name; none for any other expression. Any
// other call has neither.
func (f *aridFinder) callee(call *ast.CallExpr) (receivers []string, function string) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, ""
	}

	switch x := sel.X.(type) {
	case *ast.Ident:
		receivers = []string{x.Name}
		if path, imported := f.imports[x.Name]; imported {
			receivers = append(receivers, path, packageName(path))
		}
	case *ast.SelectorExpr:
		receivers = []string{x.Sel.Name}
	}

	return receivers, sel.Sel.Name
}

// aridBranches reports whether every branch of stmt only runs arid
// statements, so that nothing but arid code depends on its condition.
func (f *aridFinder) aridBranches(stmt *ast.IfStmt) bool {
	if !f.aridBlock(stmt.Body) {
		return false
	}

	return stmt.Else == nil || f.aridStmt(stmt.Else)
}

// aridBlock reports whether block has statements and all of them are arid.
func (f *aridFinder) aridBlock(block *ast.BlockStmt) bool {
	if len(block.List) == 0 {
		return false
	}

	return !slices.ContainsFunc(block.List, func(stmt ast.Stmt) bool { return !f.aridStmt(stmt) })
}

// aridStmt reports whether stmt is arid: an arid call, also deferred, a
// block of arid statements, or an if statement without an init statement
// whose branches are arid.
func (f *aridFinder) aridStmt(stmt ast.Stmt) bool {
	switch s := stmt.(type) {
	case *ast.ExprStmt:
		call, ok := s.X.(*ast.CallExpr)

		return ok && f.aridCall(call)
	case *ast.DeferStmt:
		return f.aridCall(s.Call)
	case *ast.BlockStmt:
		return f.aridBlock(s)
	case *ast.IfStmt:
		return s.Init == nil && f.aridBranches(s)
	}

	return false
}
