package lexer

import (
	"unicode"
	"unsafe"

	"github.com/hxkhan/evie/token"
)

// NextToken returns next token and advances the lexer
func (lex *Lexer) NextToken() (next token.Token) {
	// If we have tokens in backlog, return them first
	if lex.bi < len(lex.backlog) {
		next = lex.backlog[lex.bi]
		lex.bi++

		// Clear backlog when exhausted
		if lex.bi >= len(lex.backlog) {
			lex.backlog = lex.backlog[:0]
			lex.bi = 0
		}

		return next
	}

	return lex.compose()
}

// PeekToken returns next token without advancing the lexer
func (lex *Lexer) PeekToken() (next token.Token) {
	// If we have tokens in backlog, return them first
	if lex.bi < len(lex.backlog) {
		next = lex.backlog[lex.bi]
		return next
	}

	// weird ahh logic because compose might write to backlog
	i := len(lex.backlog)
	lex.backlog = append(lex.backlog, token.Token{})
	next = lex.compose()
	lex.backlog[i] = next
	return next
}

// newline records that the byte before cursor ended a line
func (lex *Lexer) newline() {
	lex.line++
	lex.lineStart = lex.cursor
}

// at builds a token spanning the bytes from start to the cursor
func (lex *Lexer) at(typ token.Type, lit string, start int) token.Token {
	return token.Token{Type: typ, Literal: lit, Pos: token.Pos{Line: lex.line, Column: start - lex.lineStart + 1, Length: lex.cursor - start}}
}

func (lex *Lexer) flag(lit string) token.Token {
	return token.Token{Type: token.Type(0), Literal: lit, Pos: token.Pos{Line: lex.line, Column: lex.cursor - lex.lineStart + 1, Length: 1}}
}

func (lex *Lexer) compose() token.Token {
START:
	start := lex.cursor
	current, cs := lex.advance()

	switch current {
	case iEOS:
		return lex.flag("end of source")
	case ' ', '\t', '\r':
		goto START
	case '\n':
		lex.newline()
		goto START

	case '=':
		switch {
		case lex.option('=', "==", "=") == "==":
			return lex.simple("==", start)
		case lex.option('>', "=>", "=") == "=>":
			return lex.simple("=>", start)
		}
		return lex.simple("=", start)
	case '+':
		return lex.simple(lex.option('=', "+=", "+"), start)
	case '-':
		return lex.simple(lex.option('=', "-=", "-"), start)
	case '*':
		return lex.simple(lex.option('=', "*=", "*"), start)
	case '/':
		if next, ns := lex.peek(); next == '/' {
			lex.cursor += ns
			for current, cs := lex.peek(); current != iEOS; current, cs = lex.peek() {
				lex.cursor += cs
				if current == '\n' {
					lex.newline()
					goto START
				}
			}
			goto START
		} else if next, ns := lex.peek(); next == '*' {
			lex.cursor += ns
			for current, cs := lex.peek(); current != iEOS; current, cs = lex.peek() {
				lex.cursor += cs
				switch current {
				case '*':
					if next, ns := lex.peek(); next == '/' {
						lex.cursor += ns
						goto START
					}
				case '\n':
					lex.newline()
				}
			}
			goto START
		}
		return lex.simple(lex.option('=', "/=", "/"), start)

	case '?':
		return lex.simple("?", start)
	case '!':
		return lex.simple(lex.option('=', "!=", "!"), start)
	case '%':
		return lex.simple(lex.option('=', "%=", "%"), start)

	case '>':
		return lex.simple(lex.option('=', ">=", ">"), start)
	case '<':
		return lex.simple(lex.option('=', "<=", "<"), start)

	case '|':
		return lex.simple(lex.option('|', "||", "|"), start)
	case '&':
		return lex.simple(lex.option('&', "&&", "&"), start)

	case ',':
		return lex.simple(",", start)
	case '.':
		if next1, ns1 := lex.peek(); next1 == '.' {
			if next2, ns2 := lex.get(lex.cursor + ns1); next2 == '.' {
				lex.cursor += ns1 + ns2
				return lex.simple("...", start)
			}
		}
		return lex.simple(".", start)
	case ':':
		return lex.simple(lex.option('=', ":=", ":"), start)
	case ';':
		return lex.simple(";", start)

	case '(':
		return lex.simple("(", start)
	case ')':
		return lex.simple(")", start)

	case '{':
		return lex.simple("{", start)
	case '}':
		return lex.simple("}", start)

	case '[':
		return lex.simple("[", start)
	case ']':
		return lex.simple("]", start)

	case '"':
		// get the starting position of the first character
		startPos := lex.cursor

		// get string length
		for current, cs := lex.peek(); current != '"'; current, cs = lex.peek() {
			switch current {
			case iEOS:
				// unterminated strings
				str := unsafe.String(&lex.src[startPos-1], lex.cursor-(startPos-1))
				return lex.at(token.Invalid, str, start)
			case '\\':
				// handle escaped quotations
				if next, ns := lex.get(lex.cursor + cs); next == '"' {
					lex.cursor += ns
				}
			case '\n':
				lex.cursor += cs
				lex.newline()
				continue
			}
			lex.cursor += cs
		}

		// extract string
		str := unsafe.String(&lex.src[startPos], lex.cursor-startPos)
		lex.cursor++ // add 1 for the terminating quotation
		return lex.at(token.String, str, start)

	case '`':
		lex.lexTemplateString(start)
		return lex.NextToken()

	default:
		// words
		if unicode.IsLetter(current) || current == '_' {
			// calculate word length
			for next, ns := lex.peek(); isValidNamePart(next); next, ns = lex.peek() {
				lex.cursor += ns
			}

			// extract string
			str := unsafe.String(&lex.src[start], lex.cursor-start)
			return lex.at(token.Word, str, start)
		}

		// numbers
		if unicode.IsDigit(current) {
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
			num := unsafe.String(&lex.src[start], lex.cursor-start)
			return lex.at(token.Number, num, start)
		}
	}

	return lex.at(token.Invalid, unsafe.String(&lex.src[start], cs), start)
}

// return yes if match else no; also consume if yes
func (lex *Lexer) option(match rune, yes string, no string) string {
	if next, ns := lex.peek(); next == match {
		lex.cursor += ns
		return yes
	}
	return no
}

func (lex *Lexer) simple(lit string, start int) token.Token {
	return lex.at(token.Simple, lit, start)
}

// Template string
func (lex *Lexer) lexTemplateString(tickStart int) {
	startPos := lex.cursor

	// add opening backtick
	lex.backlog = append(lex.backlog, lex.simple("`", tickStart))

	part := func() {
		// extract the string part lexed so far, if any
		if lex.cursor > startPos {
			str := unsafe.String(&lex.src[startPos], lex.cursor-startPos)
			lex.backlog = append(lex.backlog, lex.at(token.String, str, startPos))
		}
	}

	for {
		current, cs := lex.peek()

		switch current {
		case iEOS:
			// unterminated template literal
			str := unsafe.String(&lex.src[tickStart], lex.cursor-tickStart)
			lex.backlog = append(lex.backlog, lex.at(token.Invalid, str, tickStart))
			return

		case '`':
			part()
			braceStart := lex.cursor
			lex.cursor += cs // consume closing backtick
			lex.backlog = append(lex.backlog, lex.simple("`", braceStart))
			return

		case '{':
			part()
			braceStart := lex.cursor
			lex.cursor += cs // consume opening brace
			lex.backlog = append(lex.backlog, lex.simple("{", braceStart))

			// lex expression tokens until closing brace
			braceDepth := 1
			for braceDepth > 0 {
				tok := lex.compose()
				lex.backlog = append(lex.backlog, tok)

				if tok.IsSimple("{") {
					braceDepth++
				} else if tok.IsSimple("}") {
					braceDepth--
				} else if tok.IsEOS() {
					return // unterminated expression
				}
			}

			// update position for next string part
			startPos = lex.cursor

		case '\\':
			// handle escaped characters
			next, ns := lex.get(lex.cursor + cs)
			if next == '`' || next == '{' {
				lex.cursor += ns
			}
			lex.cursor += cs

		case '\n':
			lex.cursor += cs
			lex.newline()

		default:
			lex.cursor += cs
		}
	}
}

func isValidNamePart(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}
