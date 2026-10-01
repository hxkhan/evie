package parser

import (
	"strings"
	"unicode/utf8"

	"github.com/hxkhan/evie/token"
)

// unescape decodes the escape sequences of a string literal's text. extra
// lists the characters that may also be escaped in this kind of literal, such
// as the backtick and braces in a template. at is the position of the text's
// first character, used to point at a bad escape.
func (ps *parser) unescape(text string, at token.Pos, extra string) string {
	if !strings.ContainsRune(text, '\\') {
		return text
	}

	var out strings.Builder
	out.Grow(len(text))
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c != '\\' {
			out.WriteByte(c)
			continue
		}
		if i+1 == len(text) {
			ps.badEscape(text, i, at, "a backslash ends the string")
		}
		next := text[i+1]
		switch next {
		case 'n':
			out.WriteByte('\n')
		case 't':
			out.WriteByte('\t')
		case 'r':
			out.WriteByte('\r')
		case '0':
			out.WriteByte(0)
		case '\\', '"':
			out.WriteByte(next)
		default:
			if strings.IndexByte(extra, next) < 0 {
				r, _ := utf8.DecodeRuneInString(text[i+1:])
				ps.badEscape(text, i, at, "unknown escape '\\"+string(r)+"'")
			}
			out.WriteByte(next)
		}
		i++
	}
	return out.String()
}

// badEscape stops parsing at the escape that starts at byte offset i of text
func (ps *parser) badEscape(text string, i int, at token.Pos, msg string) {
	pos := at
	before := text[:i]
	if nl := strings.LastIndexByte(before, '\n'); nl >= 0 {
		pos.Line += strings.Count(before, "\n")
		pos.Column = 1 + utf8.RuneCountInString(before[nl+1:])
	} else {
		pos.Column += utf8.RuneCountInString(before)
	}
	pos.Length = 2
	err := ps.report(pos, msg)
	err.Label = "not a valid escape"
	err.Hint = `the escapes are \n \t \r \0 \\ and \"; write \\ for a backslash`
	panic(err)
}
