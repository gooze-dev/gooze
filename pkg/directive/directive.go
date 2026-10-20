// Package directive parses gooze's //gooze:ignore comments. A directive is a
// line or block comment whose text starts with the keyword gooze:ignore,
// followed by whitespace or the end of the comment, and an optional list of
// mutagen names separated by commas or whitespace. Names are case-insensitive;
// an empty list ignores every mutagen.
package directive

import (
	"go/scanner"
	"go/token"
	"strings"
	"unicode"
)

// Keyword is the text that starts a directive.
const Keyword = "gooze:ignore"

// Name is a mutagen name listed in a directive, as written, with its byte
// offsets [Start, End) in the parsed text.
type Name struct {
	Text  string
	Start int
	End   int
}

// Directive is a parsed //gooze:ignore comment. Offsets are byte offsets in
// the parsed text.
type Directive struct {
	// Keyword is the offset of Keyword.
	Keyword int
	// End is the offset where the name list ends: the end of the comment
	// without trailing whitespace and without the closing */ of a block comment.
	End int
	// Names are the listed mutagen names in source order.
	Names []Name
}

// IgnoresAll reports whether the directive lists no names and so ignores
// every mutagen.
func (d Directive) IgnoresAll() bool {
	switch len(d.Names) {
	case 0:
		return true
	default:
		return false
	}
}

// Ignores reports whether the directive ignores the named mutagen, ignoring case.
func (d Directive) Ignores(mutagen string) bool {
	if d.IgnoresAll() {
		return true
	}

	for _, n := range d.Names {
		if strings.EqualFold(n.Text, mutagen) {
			return true
		}
	}

	return false
}

// Parse parses the text of one comment, including its // or /* */ markers, as
// go/ast.Comment.Text holds it.
func Parse(text string) (Directive, bool) {
	lo, hi := trimSpace(text, 0, len(text))

	switch body := text[lo:hi]; {
	case strings.HasPrefix(body, "//"):
		lo, hi = trimSpace(text, lo+2, hi)
	case strings.HasPrefix(body, "/*"):
		lo, hi = trimSpace(text, lo+2, hi)
		if strings.HasSuffix(text[lo:hi], "*/") {
			hi -= 2
		}

		lo, hi = trimSpace(text, lo, hi)
	}

	if !strings.HasPrefix(text[lo:hi], Keyword) {
		return Directive{}, false
	}

	listStart := lo + len(Keyword)

	switch rest := text[listStart:hi]; rest {
	case "":
	default:
		if !unicode.IsSpace(rune(rest[0])) {
			return Directive{}, false
		}
	}

	return Directive{Keyword: lo, End: hi, Names: parseNames(text, listStart, hi)}, true
}

// ParseLine finds the first directive among the comments of one line of Go
// source. The line is tokenized on its own, so a line inside a multi-line
// string or block comment may be misread.
func ParseLine(line string) (Directive, bool) {
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(line))

	var s scanner.Scanner
	s.Init(file, []byte(line), func(token.Position, string) {}, scanner.ScanComments)

	for {
		pos, tok, lit := s.Scan()

		switch tok {
		case token.EOF:
			return Directive{}, false
		case token.COMMENT:
			if d, ok := Parse(lit); ok {
				return d.shift(file.Offset(pos)), true
			}
		default:
		}
	}
}

func (d Directive) shift(offset int) Directive {
	d.Keyword += offset
	d.End += offset

	for i := range d.Names {
		d.Names[i].Start += offset
		d.Names[i].End += offset
	}

	return d
}

func parseNames(text string, lo, hi int) []Name {
	names := []Name{}

	var (
		start  int
		inName bool
	)

	for i, r := range text[lo:hi] {
		separator := r == ',' || unicode.IsSpace(r)

		switch {
		case separator && inName:
			names = append(names, Name{Text: text[start : lo+i], Start: start, End: lo + i})
			inName = false
		case !separator && !inName:
			start, inName = lo+i, true
		}
	}

	if inName {
		names = append(names, Name{Text: text[start:hi], Start: start, End: hi})
	}

	return names
}

func trimSpace(text string, lo, hi int) (int, int) {
	trimmed := strings.TrimLeftFunc(text[lo:hi], unicode.IsSpace)
	lo = hi - len(trimmed)

	return lo, lo + len(strings.TrimRightFunc(trimmed, unicode.IsSpace))
}
