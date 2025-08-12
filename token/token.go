package token

import "fmt"

type Type int

type Pos int

func (pos Pos) Line() int {
	return int(pos)
}

const (
	flag Type = iota // special: used for signaling things like eos

	Simple
	Word
	String
	TemplateString
	Number
	Invalid
)

// A simple 3 tuple of (type, literal, line)
// where type is one of (Simple, Keyword, Name, String, Boolean, Number, Invalid)
type Token struct {
	Type    Type
	Literal string
	Line    Pos
}

// Checks if the token is signaling end-of-source
func (t Token) IsEOS() bool {
	return t.Type == flag && t.Literal == "eos"
}

// helper
func (t Token) IsSimple(lit string) bool {
	return t.Type == Simple && t.Literal == lit
}

func (t Token) IsWord(lit string) bool {
	return t.Type == Word && t.Literal == lit
}

func (t Token) IsAnyWord() bool {
	return t.Type == Word
}

func (t Token) IsOneOfKeywords(lit ...string) bool {
	for _, v := range lit {
		if v == t.Literal {
			return true
		}
	}
	return false
}

func (t Token) IsOneOfSimples(lit ...string) bool {
	if t.Type != Simple {
		return false
	}
	for _, v := range lit {
		if v == t.Literal {
			return true
		}
	}
	return false
}

func (t Token) String() string {
	return fmt.Sprintf("{%v, '%s'}", t.Type, t.Literal)
}

func (t Type) String() string {
	switch t {
	case flag:
		return "flag"
	case Simple:
		return "simple"
	case Word:
		return "word"
	case String:
		return "string"
	case TemplateString:
		return "template"
	case Number:
		return "number"
	case Invalid:
		return "invalid"
	}
	panic("func (Type) String() -> Unknown Type!")
}
