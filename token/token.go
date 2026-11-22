package token

import "fmt"

type Type int

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

func (p Pos) String() string {
	return fmt.Sprintf("%d:%d", p.Line, p.Column)
}

const (
	EOS Type = iota // end of source

	Simple
	Word
	String
	Number
	Invalid
)

// A token: its type, its text without delimiters, and where it is
type Token struct {
	Type    Type
	Literal string
	Pos
	NewlineBefore bool // a line break separates this token from the one before it
}

// IsEOS reports whether the token signals end-of-source
func (t Token) IsEOS() bool {
	return t.Type == EOS
}

func (t Token) IsSimple(lit string) bool {
	return t.Type == Simple && t.Literal == lit
}

func (t Token) IsWord(lit string) bool {
	return t.Type == Word && t.Literal == lit
}

// Describe names the token the way an error message should
func (t Token) Describe() string {
	switch t.Type {
	case EOS:
		return "end of source"
	case String:
		return fmt.Sprintf("string %q", t.Literal)
	case Number:
		return "number " + t.Literal
	case Invalid:
		return fmt.Sprintf("invalid token '%s'", t.Literal)
	}
	return fmt.Sprintf("'%s'", t.Literal)
}

func (t Token) String() string {
	return fmt.Sprintf("{%v, '%s'}", t.Type, t.Literal)
}

func (t Type) String() string {
	switch t {
	case EOS:
		return "eos"
	case Simple:
		return "simple"
	case Word:
		return "word"
	case String:
		return "string"
	case Number:
		return "number"
	case Invalid:
		return "invalid"
	}
	return fmt.Sprintf("unknown(%d)", t)
}
