// Package token holds the tokens the lexer hands the parser.
package token

import (
	"fmt"

	"github.com/hxkhan/evie/diag"
)

type Type int

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
	diag.Pos
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
