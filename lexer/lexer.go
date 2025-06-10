package lexer

import (
	"slices"
	"unicode"
	"unsafe"

	"github.com/hk-32/evie/token"
)

// NextToken returns next token and advances the lexer
func (lex *Lexer) NextToken() token.Token {
	lex.last = lex.next
	lex.next = lex.compose()
	return lex.last
}

// PeekToken returns next token without advancing the lexer
func (lex *Lexer) PeekToken() token.Token {
	return lex.next
}

func (lex *Lexer) flag(lit string, line int) token.Token {
	return token.Token{Type: token.Type(0), Literal: lit, Line: line}
}

func (lex *Lexer) compose() token.Token {
START:
	current, cs := lex.advance()

	switch current {
	case iEOS:
		return lex.flag("eos", lex.line)
	case ' ', '\t', '\r':
		goto START
	case '\n':
		lex.line++
		return lex.flag("\n", lex.line-1)

	case '=':
		switch {
		case lex.option('=', "==", "=") == "==":
			return lex.simple("==")
		case lex.option('>', "=>", "=") == "=>":
			return lex.simple("=>")
		}
		return lex.simple("=")
	case '+':
		return lex.simple(lex.option('=', "+=", "+"))
	case '-':
		return lex.simple(lex.option('=', "-=", "-"))
	case '*':
		return lex.simple(lex.option('=', "*=", "*"))
	case '/':
		if next, ns := lex.peek(); next == '/' {
			lex.cursor += ns
			for current, cs := lex.peek(); current != iEOS; current, cs = lex.peek() {
				lex.cursor += cs
				if current == '\n' {
					lex.line++
					goto START
				}
			}
			goto START
		} else if next, ns := lex.peek(); next == '*' {
			lex.cursor += ns
			for current, cs := lex.peek(); current != iEOS; current, cs = lex.peek() {
				lex.cursor += cs
				if current == '*' {
					if next, ns := lex.peek(); next == '/' {
						lex.cursor += ns
						goto START
					}
				} else if current == '\n' {
					lex.line++
				}
			}
			goto START
		}

		return lex.simple(lex.option('=', "/=", "/"))
	case '>':
		return lex.simple(lex.option('=', ">=", ">"))
	case '<':
		return lex.simple(lex.option('=', "<=", "<"))

	case ',':
		return lex.simple(",")
	case '.':
		return lex.simple(".")
	case ':':
		return lex.simple(lex.option('=', ":=", ":"))
	case ';':
		return lex.simple(";")

	case '(':
		return lex.simple("(")
	case ')':
		return lex.simple(")")

	case '{':
		return lex.simple("{")
	case '}':
		return lex.simple("}")

	case '[':
		return lex.simple("[")
	case ']':
		return lex.simple("]")

	case '"':
		// get the starting position of the first character
		startPos := lex.cursor

		// get string length
		for current, cs := lex.peek(); current != '"'; current, cs = lex.peek() {
			if current == iEOS {
				// unterminated strings are invalid
				str := unsafe.String(&lex.src[startPos-1], lex.cursor-(startPos-1))
				return token.Token{Type: token.Invalid, Literal: str, Line: lex.line}
			} else if current == '\\' {
				// to make sure we don't stop lexing the string on quotations prepended by backslash
				if next, ns := lex.get(lex.cursor + cs); next == '"' {
					lex.cursor += ns
				}
			}
			lex.cursor += cs
		}

		// extract string
		str := unsafe.String(&lex.src[startPos], lex.cursor-startPos)
		lex.cursor++ // add 1 for the terminating quotation
		return token.Token{Type: token.String, Literal: str, Line: lex.line}

	default: // words & numbers
		if unicode.IsLetter(current) {
			// get the starting position of the first letter
			startPos := lex.cursor - cs

			// calculate word length
			for next, ns := lex.peek(); isValidNamePart(next); next, ns = lex.peek() {
				lex.cursor += ns
			}

			// extract string
			str := unsafe.String(&lex.src[startPos], lex.cursor-startPos)
			return token.Token{Type: lex.wordCategory(str), Literal: str, Line: lex.line}
		}

		if unicode.IsDigit(current) {
			// get the starting position of the first digit
			startPos := lex.cursor - cs

			for next, ns := lex.peek(); next != iEOS; next, ns = lex.peek() {
				if unicode.IsDigit(next) {
					lex.cursor += ns
					continue
				} else if next == '.' {
					// don't swallow the dot immediately; check what is after it
					if afterDot, ads := lex.get(lex.cursor + ns); unicode.IsDigit(afterDot) {
						lex.cursor += ns + ads
						continue
					}
				}

				break
			}
			// extract number content
			num := unsafe.String(&lex.src[startPos], lex.cursor-startPos)
			return token.Token{Type: token.Number, Literal: num, Line: lex.line}
		}
	}

	return token.Token{Type: token.Invalid, Literal: unsafe.String(&lex.src[lex.cursor-cs], 1), Line: lex.line}
}

// return yes if match else no; also consume if yes
func (lex *Lexer) option(match rune, yes string, no string) string {
	if next, ns := lex.peek(); next == match {
		lex.cursor += ns
		return yes
	}
	return no
}

func (lex *Lexer) simple(lit string) token.Token {
	return token.Token{Type: token.Simple, Literal: lit, Line: lex.line}
}

// Checks if the last token is a valid way to terminate a line
/* func (lex *Lexer) isLastValidTermination() bool {
	switch lex.last.Type {
	case token.Name, token.String, token.Number:
		return true
	case token.Simple:
		if lex.last.Literal == ")" {
			return true
		}
	case token.Keyword:
		switch lex.last.Literal {
		case "return":
			return true
		}
	}
	return false
} */

func (lex *Lexer) wordCategory(word string) token.Type {
	if slices.Contains(keywords, word) {
		return token.Keyword
	}

	return token.Name
}

func isValidNamePart(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}
