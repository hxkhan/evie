package lexer

import (
	"unicode/utf8"

	"github.com/hxkhan/evie/token"
)

const iEOS rune = 0x03 // 0x03 = End of Source

// all recognized keywords
var keywords = []string{
	"package",
	"null", "true", "false",
	"fn", "return",
	"go", "await", "echo",
	"if", "else",
}

type Lexer struct {
	src    []byte    // the whole input
	cursor int       // current position in source
	line   token.Pos // current line number

	last token.Token // last token returned
	next token.Token // next token to be returned
}

func New(input []byte) *Lexer {
	lex := &Lexer{src: input, cursor: 0, line: 1}
	lex.next = lex.compose()
	return lex
}

// decodes a rune starting on the given index, use carefully so not to step in the middle of a rune
func (lex *Lexer) get(n int) (r rune, size int) {
	if n < len(lex.src) {
		//return utf8.DecodeRune(lex.src[lex.cursor:])

		if b := lex.src[n]; b < utf8.RuneSelf {
			return rune(b), 1
		}
		return utf8.DecodeRune(lex.src[n:])
	}
	return iEOS, 0
}

// returns the next rune without advancing the cursor
func (lex *Lexer) peek() (r rune, size int) {
	if lex.cursor < len(lex.src) {
		//return utf8.DecodeRune(lex.src[lex.cursor:])

		if b := lex.src[lex.cursor]; b < utf8.RuneSelf {
			return rune(b), 1
		}
		return utf8.DecodeRune(lex.src[lex.cursor:])
	}
	return iEOS, 0
}

// returns the next rune and advances the cursor
func (lex *Lexer) advance() (r rune, size int) {
	if lex.cursor < len(lex.src) {
		/* r, size = utf8.DecodeRune(lex.src[lex.cursor:])
		lex.cursor += size
		return r, size */

		if b := lex.src[lex.cursor]; b < utf8.RuneSelf {
			lex.cursor++
			return rune(b), 1
		}

		r, size := utf8.DecodeRune(lex.src[lex.cursor:])
		lex.cursor += size
		return r, size
	}
	return iEOS, 0
}
