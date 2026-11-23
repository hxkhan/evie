package analysis

import (
	"bytes"
	"unicode/utf8"

	"github.com/hxkhan/evie/diag"
)

// Position is a place in a document the way editors count it: a zero-based
// line, and a zero-based character offset in UTF-16 code units
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range is the text between two positions; End is just past it
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location is a range in a file of the workspace
type Location struct {
	Path  string `json:"path"`
	Range Range  `json:"range"`
}

// utf16Len counts the UTF-16 code units of UTF-8 text
func utf16Len(b []byte) int {
	n := 0
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		b = b[size:]
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// toPosition converts a 1-based line and byte column of src
func toPosition(src *diag.Source, line, col int) Position {
	if line < 1 {
		return Position{}
	}
	text, ok := src.Line(line)
	if !ok {
		return Position{Line: line - 1}
	}
	n := min(max(col-1, 0), len(text))
	return Position{Line: line - 1, Character: utf16Len(text[:n])}
}

// toRange converts a span of src; an unknown span is the start of the file
func toRange(src *diag.Source, p diag.Pos) Range {
	if !p.Known() {
		return Range{}
	}
	return Range{toPosition(src, p.Line, p.Column), toPosition(src, p.Line, p.Column+max(p.Length, 1))}
}

// spanRange converts the text from one span's start to another's end
func spanRange(src *diag.Source, from, to diag.Pos) Range {
	r := toRange(src, from)
	if to.Known() {
		r.End = toRange(src, to).End
	}
	return r
}

// fromPosition converts a position in text to a 1-based line and byte column
func fromPosition(text []byte, pos Position) (line, col int) {
	lines := bytes.Split(text, []byte{'\n'})
	if pos.Line < 0 || pos.Line >= len(lines) {
		return pos.Line + 1, 1
	}
	l := lines[pos.Line]
	units, i := 0, 0
	for i < len(l) && units < pos.Character {
		r, size := utf8.DecodeRune(l[i:])
		if r >= 0x10000 {
			units += 2
		} else {
			units++
		}
		i += size
	}
	return pos.Line + 1, i + 1
}

// before reports whether line:col comes before p starts
func before(line, col int, p diag.Pos) bool {
	return line < p.Line || line == p.Line && col < p.Column
}
