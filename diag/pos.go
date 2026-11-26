package diag

import "fmt"

// Pos is a span in the source: where it starts, in 1-based line and byte
// column, and how many bytes of that line it covers. A zero Pos is unknown.
type Pos struct {
	Line   int
	Column int
	Length int
}

// Where lets any node that embeds a Pos report it
func (p Pos) Where() Pos {
	return p
}

// Known reports whether the position was recorded
func (p Pos) Known() bool {
	return p.Line > 0
}

// End is the column just past the span
func (p Pos) End() int {
	return p.Column + max(p.Length, 1)
}

// To extends the span to the end of another one on the same line; a span on
// another line is left as it is, since excerpts show one line
func (p Pos) To(end Pos) Pos {
	if !p.Known() {
		return end
	}
	if end.Known() && end.Line == p.Line && end.End() > p.End() {
		p.Length = end.End() - p.Column
	}
	return p
}

// String returns the position as "line:column"
func (p Pos) String() string {
	return fmt.Sprintf("%d:%d", p.Line, p.Column)
}
