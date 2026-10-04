// Package diag renders errors the way a compiler should: the message, the
// offending line with the span underlined and labelled, related places, a
// hint, and for runtime errors the chain of calls that led there.
//
//	error: cannot add a number and a string
//	  --> examples/errors.ev:14:16
//	   |
//	14 |     b := catch(5 + "10")
//	   |                ^^^^^^^^ number + string
//	   |
//	   = help: convert one side with string(...) or number(...)
package diag

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/hxkhan/evie/token"
)

// Source is a script's text and the name it is reported under
type Source struct {
	Name string
	Text []byte

	lines [][]byte
}

// NewSource names a script
func NewSource(name string, text []byte) *Source {
	return &Source{Name: name, Text: text}
}

// Line returns the text of a 1-based line without its newline
func (s *Source) Line(n int) ([]byte, bool) {
	if s == nil {
		return nil, false
	}
	if s.lines == nil {
		s.lines = bytes.Split(s.Text, []byte{'\n'})
	}
	if n < 1 || n > len(s.lines) {
		return nil, false
	}
	return bytes.TrimRight(s.lines[n-1], "\r"), true
}

// Note is a secondary place worth showing, such as where a function was defined
type Note struct {
	Source *Source
	Pos    token.Pos
	Text   string
}

// Frame is one function on the call stack and where execution was inside it
type Frame struct {
	Function string
	Source   *Source
	Pos      token.Pos
}

// Report is everything known about one error
type Report struct {
	Message string
	Source  *Source
	Pos     token.Pos // the primary span; zero when unknown
	Label   string    // shown under the primary span
	Notes   []Note
	Hint    string
	Trace   []Frame // innermost first
}

// Diagnostic returns the report itself, so any error that embeds one can be
// found with errors.As and rendered
func (r *Report) Diagnostic() *Report {
	return r
}

// Location names the file and position, or "" when unknown
func (r *Report) Location() string {
	if !r.Pos.Known() {
		return ""
	}
	name := "<script>"
	if r.Source != nil && r.Source.Name != "" {
		name = r.Source.Name
	}
	return fmt.Sprintf("%s:%d:%d", name, r.Pos.Line, r.Pos.Column)
}

// Error is the one-line form: the message, then the location in parentheses
func (r *Report) Error() string {
	if loc := r.Location(); loc != "" {
		return r.Message + " (" + loc + ")"
	}
	return r.Message
}

// Palette holds the escape sequences a rendering uses; the zero value renders plain text
type Palette struct {
	Error, Bold, Gutter, Caret, Hint, Reset string
}

// Colors is the palette for terminals that understand ANSI escapes
var Colors = Palette{
	Error:  "\x1b[1;31m",
	Bold:   "\x1b[1m",
	Gutter: "\x1b[1;34m",
	Caret:  "\x1b[1;31m",
	Hint:   "\x1b[1;36m",
	Reset:  "\x1b[0m",
}

// Render draws the report over several lines, with the excerpt when the
// source is known, using the palette for emphasis
func (r *Report) Render(p Palette) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%serror%s: %s%s%s\n", p.Error, p.Reset, p.Bold, r.Message, p.Reset)

	width := len(strconv.Itoa(r.Pos.Line))
	for _, n := range r.Notes {
		width = max(width, len(strconv.Itoa(n.Pos.Line)))
	}
	pad := strings.Repeat(" ", width)
	gutter := func(s string) string { return p.Gutter + s + p.Reset }

	if loc := r.Location(); loc != "" {
		fmt.Fprintf(&b, "%s%s %s\n", pad, gutter("-->"), loc)
	}

	excerpt := func(src *Source, pos token.Pos, label string, caret string) bool {
		line, ok := src.Line(pos.Line)
		if !ok {
			return false
		}
		text := strings.ReplaceAll(string(line), "\t", "    ")
		// a column counts tabs before it as four columns too
		col := pos.Column - 1 + 3*strings.Count(string(line[:min(pos.Column-1, len(line))]), "\t")
		length := max(pos.Length, 1)
		if col+length > len(text) {
			length = max(len(text)-col, 1)
		}
		fmt.Fprintf(&b, "%s %s\n", pad, gutter("|"))
		fmt.Fprintf(&b, "%s %s %s\n", gutter(fmt.Sprintf("%*d", width, pos.Line)), gutter("|"), text)
		fmt.Fprintf(&b, "%s %s %s", pad, gutter("|"), strings.Repeat(" ", col))
		b.WriteString(p.Caret + strings.Repeat(caret, length) + p.Reset)
		if label != "" {
			b.WriteString(" " + p.Caret + label + p.Reset)
		}
		b.WriteByte('\n')
		return true
	}

	shown := false
	if r.Pos.Known() {
		shown = excerpt(r.Source, r.Pos, r.Label, "^")
	}
	for _, n := range r.Notes {
		if n.Pos.Known() {
			// a note in another file says which file first
			if n.Source != nil && n.Source != r.Source && n.Source.Name != "" {
				if shown {
					fmt.Fprintf(&b, "%s %s\n", pad, gutter("|"))
				}
				fmt.Fprintf(&b, "%s%s %s:%d:%d\n", pad, gutter(":::"), n.Source.Name, n.Pos.Line, n.Pos.Column)
			}
			shown = excerpt(n.Source, n.Pos, n.Text, "-") || shown
		} else if n.Text != "" {
			fmt.Fprintf(&b, "%s %s note: %s\n", pad, gutter("="), n.Text)
		}
	}
	if shown {
		fmt.Fprintf(&b, "%s %s\n", pad, gutter("|"))
	}

	if r.Hint != "" {
		fmt.Fprintf(&b, "%s %s %shelp%s: %s\n", pad, gutter("="), p.Hint, p.Reset, r.Hint)
	}

	if len(r.Trace) > 0 {
		fmt.Fprintf(&b, "%s %s trace:", pad, gutter("="))
		for i, f := range r.Trace {
			name := f.Function
			if name == "" {
				name = "<anonymous>"
			}
			where := ""
			if f.Pos.Known() {
				where = " at " + (&Report{Source: f.Source, Pos: f.Pos}).Location()
			}
			if i == 0 {
				fmt.Fprintf(&b, " %s%s\n", name, where)
			} else {
				fmt.Fprintf(&b, "%s          %s%s\n", pad, name, where)
			}
		}
	}
	return b.String()
}
