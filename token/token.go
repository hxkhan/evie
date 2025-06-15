package token

import "fmt"

type Type int

const (
	flag Type = iota // special: used for signaling things like eos

	Simple
	Keyword
	Name
	String
	Number
	Invalid
)

// A simple 3 tuple of (type, literal, line)
// where type is one of (Simple, Keyword, Name, String, Boolean, Number, Invalid)
type Token struct {
	Type    Type
	Literal string
	Line    int
}

// Checks if the token is signaling end-of-source
func (t Token) IsEOS() bool {
	return t.Type == flag && t.Literal == "eos"
}

func (t Token) IsNewLine() bool {
	return t.Type == flag && t.Literal == "\n"
}

// helper
func (t Token) IsSimple(lit string) bool {
	return t.Type == Simple && t.Literal == lit
}

// helper
func (t Token) IsKeyword(lit string) bool {
	return t.Type == Keyword && t.Literal == lit
}

func (t Token) IsName(lit string) bool {
	return t.Type == Name && t.Literal == lit
}

func (t Token) IsOneOfKeywords(lit ...string) bool {
	if t.Type != Keyword {
		return false
	}
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
	if t.IsNewLine() {
		return fmt.Sprintf("{%v, '\\n'}", t.Type)
	}
	return fmt.Sprintf("{%v, '%s'}", t.Type, t.Literal)
}

func (t Type) String() string {
	switch t {
	case flag:
		return "flag"
	case Simple:
		return "simple"
	case Keyword:
		return "keyword"
	case Name:
		return "name"
	case String:
		return "string"
	case Number:
		return "number"
	case Invalid:
		return "invalid"
	}
	panic("func (Type) String() -> Unknown Type!")
}
