package lexer

import (
	"unicode"
	"unsafe"

	"hxkhan.dev/evie/token"
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

func (lex *Lexer) flag(lit string, line token.Pos) token.Token {
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
		goto START

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
				switch current {
				case '*':
					if next, ns := lex.peek(); next == '/' {
						lex.cursor += ns
						goto START
					}
				case '\n':
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

	case '|':
		return lex.simple(lex.option('|', "||", "|"))
	case '&':
		return lex.simple(lex.option('&', "&&", "&"))

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
			switch current {
			case iEOS:
				// unterminated strings
				str := unsafe.String(&lex.src[startPos-1], lex.cursor-(startPos-1))
				return token.Token{Type: token.Invalid, Literal: str, Line: lex.line}
			case '\\':
				// handle escaped quotations
				if next, ns := lex.get(lex.cursor + cs); next == '"' {
					lex.cursor += ns
				}
			case '\n':
				lex.line++
			}
			lex.cursor += cs
		}

		// extract string
		str := unsafe.String(&lex.src[startPos], lex.cursor-startPos)
		lex.cursor++ // add 1 for the terminating quotation
		return token.Token{Type: token.String, Literal: str, Line: lex.line}

	case '`':
		lex.lexTemplateString()
		return lex.NextToken()

	default:
		// words
		if unicode.IsLetter(current) || current == '_' {
			// get the starting position of the first letter
			startPos := lex.cursor - cs

			// calculate word length
			for next, ns := lex.peek(); isValidNamePart(next); next, ns = lex.peek() {
				lex.cursor += ns
			}

			// extract string
			str := unsafe.String(&lex.src[startPos], lex.cursor-startPos)
			return token.Token{Type: token.Word, Literal: str, Line: lex.line}
		}

		// numbers
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

// Template string
func (lex *Lexer) lexTemplateString() {
	startPos := lex.cursor
	startLine := lex.line

	// add opening backtick
	lex.backlog = append(lex.backlog, lex.simple("`"))

	for {
		current, cs := lex.peek()

		switch current {
		case iEOS:
			// unterminated template literal
			str := unsafe.String(&lex.src[startPos-1], lex.cursor-(startPos-1))
			lex.backlog = append(lex.backlog, token.Token{Type: token.Invalid, Literal: str, Line: lex.line})
			return

		case '`':
			// extract final string part
			if lex.cursor > startPos {
				str := unsafe.String(&lex.src[startPos], lex.cursor-startPos)
				lex.backlog = append(lex.backlog, token.Token{Type: token.String, Literal: str, Line: startLine})
			}

			lex.cursor += cs // consume closing backtick
			lex.backlog = append(lex.backlog, lex.simple("`"))
			return

		case '{':
			// extract string part before interpolation
			if lex.cursor > startPos {
				str := unsafe.String(&lex.src[startPos], lex.cursor-startPos)
				lex.backlog = append(lex.backlog, token.Token{Type: token.String, Literal: str, Line: startLine})
			}

			lex.cursor += cs // consume opening brace
			lex.backlog = append(lex.backlog, lex.simple("{"))

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
			startLine = lex.line

		case '\\':
			// handle escaped characters
			next, ns := lex.get(lex.cursor + cs)
			if next == '`' || next == '{' {
				lex.cursor += ns
			}
			lex.cursor += cs

		case '\n':
			lex.line++
			lex.cursor += cs

		default:
			lex.cursor += cs
		}
	}
}

func isValidNamePart(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}
