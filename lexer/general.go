package lexer

import (
	"unicode/utf8"

	"github.com/hxkhan/evie/token"
)

const iEOS rune = -1 // out-of-band End of Source sentinel

type Lexer struct {
	src       []byte // the whole input
	cursor    int    // current position in source
	line      int    // current line number
	lineStart int    // offset of the first byte of the current line

	backlog []token.Token // backlog of tokens to return
	bi      int           // backlog index
	depth   int           // how many template literals the cursor is inside
}

// maxTemplateDepth bounds how deeply template literals may nest. Templates are
// lexed eagerly and recursively, so without a bound deep nesting would overflow
// the Go stack before the parser, which allows less nesting than this and
// reports it as an error, ever saw a token.
const maxTemplateDepth = 512

func New(input []byte) *Lexer {
	return &Lexer{src: input, cursor: 0, line: 1}
}

// decodes a rune starting on the given index, use carefully so not to step in the middle of a rune
func (lex *Lexer) get(n int) (r rune, size int) {
	if n < len(lex.src) {
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
