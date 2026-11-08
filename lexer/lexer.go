package lexer

import (
	"unicode"
	"unsafe"

	"github.com/hxkhan/evie/token"
)

// NextToken returns next token and advances the lexer
func (lex *Lexer) NextToken() (next token.Token) {
	if lex.bi >= len(lex.backlog) {
		lex.fill()
	}
	next = lex.backlog[lex.bi]
	lex.bi++
	if lex.bi >= len(lex.backlog) {
		lex.backlog = lex.backlog[:0]
		lex.bi = 0
	}
	return next
}

// PeekToken returns next token without advancing the lexer
func (lex *Lexer) PeekToken() (next token.Token) {
	if lex.bi >= len(lex.backlog) {
		lex.fill()
	}
	return lex.backlog[lex.bi]
}

// fill lexes the next token (or sequence of tokens for a template literal) into the backlog
func (lex *Lexer) fill() {
	lex.backlog = lex.backlog[:0]
	lex.bi = 0
	lex.composeInto(&lex.backlog)
}

// newline records that the byte before cursor ended a line
func (lex *Lexer) newline() {
	lex.line++
	lex.lineStart = lex.cursor
}

// at builds a token spanning the bytes from start to the cursor on the current line
func (lex *Lexer) at(typ token.Type, lit string, start int) token.Token {
	return token.Token{Type: typ, Literal: lit, Pos: token.Pos{Line: lex.line, Column: start - lex.lineStart + 1, Length: lex.cursor - start}}
}

// atSpan builds a token whose span started at (line, col), clamping length to the opening line if multiline
func (lex *Lexer) atSpan(typ token.Type, lit string, start, startLine, startCol, firstNL int) token.Token {
	length := lex.cursor - start
	if firstNL >= 0 {
		length = max(firstNL-start, 1)
	}
	return token.Token{Type: typ, Literal: lit, Pos: token.Pos{Line: startLine, Column: startCol, Length: length}}
}

func (lex *Lexer) flag(lit string) token.Token {
	return token.Token{Type: token.Type(0), Literal: lit, Pos: token.Pos{Line: lex.line, Column: lex.cursor - lex.lineStart + 1, Length: 1}}
}

func (lex *Lexer) composeInto(dst *[]token.Token) {
START:
	start := lex.cursor
	startLine := lex.line
	startCol := start - lex.lineStart + 1
	current, cs := lex.advance()

	switch current {
	case iEOS:
		*dst = append(*dst, lex.flag("end of source"))
		return
	case ' ', '\t', '\r':
		goto START
	case '\n':
		lex.newline()
		goto START

	case '=':
		switch {
		case lex.option('=', "==", "=") == "==":
			*dst = append(*dst, lex.simple("==", start))
			return
		case lex.option('>', "=>", "=") == "=>":
			*dst = append(*dst, lex.simple("=>", start))
			return
		}
		*dst = append(*dst, lex.simple("=", start))
		return
	case '+':
		*dst = append(*dst, lex.simple(lex.option('=', "+=", "+"), start))
		return
	case '-':
		*dst = append(*dst, lex.simple(lex.option('=', "-=", "-"), start))
		return
	case '*':
		*dst = append(*dst, lex.simple(lex.option('=', "*=", "*"), start))
		return
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
			*dst = append(*dst, token.Token{Type: token.Invalid, Literal: "/*", Pos: token.Pos{Line: startLine, Column: startCol, Length: 2}})
			return
		}
		*dst = append(*dst, lex.simple(lex.option('=', "/=", "/"), start))
		return

	case '?':
		*dst = append(*dst, lex.simple("?", start))
		return
	case '!':
		*dst = append(*dst, lex.simple(lex.option('=', "!=", "!"), start))
		return
	case '%':
		*dst = append(*dst, lex.simple(lex.option('=', "%=", "%"), start))
		return

	case '>':
		*dst = append(*dst, lex.simple(lex.option('=', ">=", ">"), start))
		return
	case '<':
		*dst = append(*dst, lex.simple(lex.option('=', "<=", "<"), start))
		return

	case '|':
		*dst = append(*dst, lex.simple(lex.option('|', "||", "|"), start))
		return
	case '&':
		*dst = append(*dst, lex.simple(lex.option('&', "&&", "&"), start))
		return

	case ',':
		*dst = append(*dst, lex.simple(",", start))
		return
	case '.':
		if next1, ns1 := lex.peek(); next1 == '.' {
			if next2, ns2 := lex.get(lex.cursor + ns1); next2 == '.' {
				lex.cursor += ns1 + ns2
				*dst = append(*dst, lex.simple("...", start))
				return
			}
		}
		*dst = append(*dst, lex.simple(".", start))
		return
	case ':':
		*dst = append(*dst, lex.simple(lex.option('=', ":=", ":"), start))
		return
	case ';':
		*dst = append(*dst, lex.simple(";", start))
		return

	case '(':
		*dst = append(*dst, lex.simple("(", start))
		return
	case ')':
		*dst = append(*dst, lex.simple(")", start))
		return

	case '{':
		*dst = append(*dst, lex.simple("{", start))
		return
	case '}':
		*dst = append(*dst, lex.simple("}", start))
		return

	case '[':
		*dst = append(*dst, lex.simple("[", start))
		return
	case ']':
		*dst = append(*dst, lex.simple("]", start))
		return

	case '"':
		// get the starting position of the first character
		startPos := lex.cursor
		firstNL := -1

		// get string length
		for current, cs := lex.peek(); current != '"'; current, cs = lex.peek() {
			switch current {
			case iEOS:
				// unterminated strings
				str := unsafe.String(&lex.src[startPos-1], lex.cursor-(startPos-1))
				*dst = append(*dst, lex.atSpan(token.Invalid, str, start, startLine, startCol, firstNL))
				return
			case '\\':
				// advance past both the backslash and the escaped character
				lex.cursor += cs
				if next, ns := lex.peek(); next != iEOS {
					if next == '\n' {
						if firstNL < 0 {
							firstNL = lex.cursor
						}
						lex.cursor += ns
						lex.newline()
					} else {
						lex.cursor += ns
					}
				}
				continue
			case '\n':
				if firstNL < 0 {
					firstNL = lex.cursor
				}
				lex.cursor += cs
				lex.newline()
				continue
			}
			lex.cursor += cs
		}

		// extract string
		str := unsafe.String(&lex.src[startPos], lex.cursor-startPos)
		lex.cursor++ // add 1 for the terminating quotation
		*dst = append(*dst, lex.atSpan(token.String, str, start, startLine, startCol, firstNL))
		return

	case '`':
		lex.lexTemplateInto(start, startLine, startCol, dst)
		return

	default:
		// words
		if unicode.IsLetter(current) || current == '_' {
			// calculate word length
			for next, ns := lex.peek(); isValidNamePart(next); next, ns = lex.peek() {
				lex.cursor += ns
			}

			// extract string
			str := unsafe.String(&lex.src[start], lex.cursor-start)
			*dst = append(*dst, lex.at(token.Word, str, start))
			return
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
			*dst = append(*dst, lex.at(token.Number, num, start))
			return
		}
	}

	*dst = append(*dst, lex.at(token.Invalid, unsafe.String(&lex.src[start], cs), start))
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

// lexTemplateInto appends all tokens of a template literal (including nested templates) into dst
func (lex *Lexer) lexTemplateInto(tickStart, tickLine, tickCol int, dst *[]token.Token) {
	startPos := lex.cursor
	partLine := lex.line
	partCol := startPos - lex.lineStart + 1
	partFirstNL := -1

	// add opening backtick
	*dst = append(*dst, token.Token{Type: token.Simple, Literal: "`", Pos: token.Pos{Line: tickLine, Column: tickCol, Length: 1}})

	part := func() {
		// extract the string part lexed so far, if any
		if lex.cursor > startPos {
			str := unsafe.String(&lex.src[startPos], lex.cursor-startPos)
			*dst = append(*dst, lex.atSpan(token.String, str, startPos, partLine, partCol, partFirstNL))
		}
	}

	for {
		current, cs := lex.peek()

		switch current {
		case iEOS:
			// unterminated template literal
			str := unsafe.String(&lex.src[tickStart], lex.cursor-tickStart)
			firstNL := -1
			if lex.line > tickLine {
				firstNL = tickStart + 1
			}
			*dst = append(*dst, lex.atSpan(token.Invalid, str, tickStart, tickLine, tickCol, firstNL))
			return

		case '`':
			part()
			braceStart := lex.cursor
			lex.cursor += cs // consume closing backtick
			*dst = append(*dst, lex.simple("`", braceStart))
			return

		case '{':
			part()
			braceStart := lex.cursor
			lex.cursor += cs // consume opening brace
			*dst = append(*dst, lex.simple("{", braceStart))

			// lex expression tokens until closing brace; nested templates append directly into dst
			braceDepth := 1
			for braceDepth > 0 {
				before := len(*dst)
				lex.composeInto(dst)
				for _, tok := range (*dst)[before:] {
					if tok.IsSimple("{") {
						braceDepth++
					} else if tok.IsSimple("}") {
						braceDepth--
					} else if tok.IsEOS() {
						return // unterminated expression
					}
				}
			}

			// update position for next string part
			startPos = lex.cursor
			partLine = lex.line
			partCol = startPos - lex.lineStart + 1
			partFirstNL = -1

		case '\\':
			// advance past both the backslash and the escaped character
			lex.cursor += cs
			if next, ns := lex.peek(); next != iEOS {
				if next == '\n' {
					if partFirstNL < 0 {
						partFirstNL = lex.cursor
					}
					lex.cursor += ns
					lex.newline()
				} else {
					lex.cursor += ns
				}
			}

		case '\n':
			if partFirstNL < 0 {
				partFirstNL = lex.cursor
			}
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
