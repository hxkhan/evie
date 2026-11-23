package analysis

import (
	"slices"
	"strings"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/internal/lexer"
	"github.com/hxkhan/evie/internal/token"
	"github.com/hxkhan/evie/vm"
)

// SignatureHelp shows the parameters of the call the cursor is in, with the
// one being written marked
type SignatureHelp struct {
	Signatures      []SignatureInformation `json:"signatures"`
	ActiveSignature int                    `json:"activeSignature"`
	ActiveParameter int                    `json:"activeParameter"`
}

// SignatureInformation is a callable's signature
type SignatureInformation struct {
	Label         string                 `json:"label"`
	Documentation string                 `json:"documentation,omitempty"` // Markdown
	Parameters    []ParameterInformation `json:"parameters"`
}

// ParameterInformation is a parameter, by where its name is in the label,
// counted in UTF-16 code units
type ParameterInformation struct {
	Label [2]int `json:"label"`
}

// call is the call the cursor is in: who is called, and how many arguments
// come before the one being written
type call struct {
	callee []string // as in ["io", "println"]
	commas int
	line   int // where the callee is
}

// openCall finds the innermost call of text that is still open at line:col,
// lexing the text up to there, so it works on code that does not parse
func openCall(text []byte, line, col int) (call, bool) {
	type open struct {
		bracket string
		call    call
		isCall  bool
	}
	var stack []open
	var prev []token.Token // the tokens just before, to find a callee
	lex := lexer.New(text)
	for {
		tok := lex.NextToken()
		if tok.IsEOS() || tok.Pos.Line > line || tok.Pos.Line == line && tok.Pos.Column >= col {
			break
		}
		if tok.Type == token.Simple {
			switch tok.Literal {
			case "(":
				callee, isCall := calleeOf(prev)
				stack = append(stack, open{"(", call{callee: callee, line: tok.Pos.Line}, isCall})
			case "[", "{":
				stack = append(stack, open{bracket: tok.Literal})
			case ")", "]", "}":
				if len(stack) > 0 {
					stack = stack[:len(stack)-1]
				}
			case ",":
				if n := len(stack); n > 0 && stack[n-1].bracket == "(" {
					stack[n-1].call.commas++
				}
			}
		}
		prev = append(prev, tok)
		if len(prev) > 8 {
			prev = prev[1:]
		}
	}
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i].bracket == "(" {
			return stack[i].call, stack[i].isCall
		}
		if stack[i].bracket == "{" {
			break // a block or object between: not inside the call's arguments
		}
	}
	return call{}, false
}

// calleeOf reads the name chain that ends the tokens before a '(', as in
// io.println or greet; a keyword such as if or fn is no call
func calleeOf(prev []token.Token) ([]string, bool) {
	var chain []string
	i := len(prev) - 1
	for i >= 0 {
		if prev[i].Type != token.Word || slices.Contains(keywords, prev[i].Literal) && prev[i].Literal != "this" {
			break
		}
		chain = append([]string{prev[i].Literal}, chain...)
		if i == 0 || !prev[i-1].IsSimple(".") {
			break
		}
		i -= 2
	}
	return chain, len(chain) > 0 && chain[len(chain)-1] != "this"
}

// SignatureHelp shows the parameters of the call that the cursor at pos is in,
// where text is the document as it is now, which may not parse
func (w *Workspace) SignatureHelp(file string, pos Position, text string) (SignatureHelp, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	line, col := fromPosition([]byte(text), pos)
	c, ok := openCall([]byte(text), line, col)
	if !ok {
		return SignatureHelp{}, false
	}
	s := w.names(file)
	name, params, variadic, doc, ok := s.callable(file, c)
	if !ok {
		return SignatureHelp{}, false
	}
	sig := SignatureInformation{Documentation: doc}
	label := name + "("
	for i, p := range params {
		if i > 0 {
			label += ", "
		}
		start := utf16Len([]byte(label))
		label += p
		sig.Parameters = append(sig.Parameters, ParameterInformation{Label: [2]int{start, utf16Len([]byte(label))}})
	}
	sig.Label = label + ")"
	active := c.commas
	if n := len(params); n > 0 && active >= n {
		active = n - 1 // more arguments: the variadic parameter takes them, or there are too many
		if !variadic {
			active = n
		}
	}
	return SignatureHelp{Signatures: []SignatureInformation{sig}, ActiveParameter: active}, true
}

// callable finds what a call calls: its name, parameters, whether the last
// takes any number of arguments, and a description
func (s *snapshot) callable(file string, c call) (name string, params []string, variadic bool, doc string, ok bool) {
	switch {
	case len(c.callee) == 2 && c.callee[0] == "this":
		// a method of the class around the call
		for _, m := range s.classMembers(file, c.line) {
			if m.Label == c.callee[1] && m.Kind == completionMethod {
				return s.declared(file, c.line, c.callee[1], true)
			}
		}
	case len(c.callee) == 2:
		pkg := s.importNamed(file, c.callee[0])
		if pkg == nil {
			return
		}
		member := c.callee[1]
		if !pkg.Compiled() {
			m, found := pkg.Describe(member)
			if !found {
				return
			}
			name, params, variadic = parseSignature(m.Signature)
			doc = m.Doc
			if m.Yields {
				doc += "\n\n**yields**: other tasks run while it waits"
			}
			return c.callee[0] + "." + name, params, variadic, doc, name != ""
		}
		// a function of an Evie package: find where it is declared
		for f, refs := range s.refs {
			for _, r := range refs {
				if r.Declares && r.Name == member && isTopLevel(r.Kind) && parserPackage(s, f) == pkg.Name() {
					name, params, variadic, doc, ok = s.signatureAt(f, r.Pos)
					return c.callee[0] + "." + name, params, variadic, doc, ok
				}
			}
		}
	case len(c.callee) == 1:
		return s.declared(file, c.line, c.callee[0], false)
	}
	return
}

// declared finds the callable a name refers to on a line of file: the
// nearest declaration of it before the line in the file, or one of the package
func (s *snapshot) declared(file string, line int, name string, method bool) (string, []string, bool, string, bool) {
	var best *ref
	for _, r := range s.refs[file] {
		if r.Declares && r.Name == name && (r.Kind != vm.NameField || method) && r.Pos.Line <= line {
			if best == nil || r.Pos.Line >= best.Pos.Line {
				best = &r
			}
		}
	}
	if best == nil {
		pkg := s.packageOf(file)
		for f, refs := range s.refs {
			for _, r := range refs {
				if r.Declares && r.Name == name && isTopLevel(r.Kind) && s.packageOf(f) == pkg {
					best = &r
				}
			}
		}
	}
	if best == nil {
		return "", nil, false, "", false
	}
	return s.signatureAt(best.path, best.Pos)
}

// signatureAt reads the parameters of what is declared at pos of file: a
// function, a method, a class (whose fields its constructor takes), or a
// binding to a function literal
func (s *snapshot) signatureAt(file string, pos diag.Pos) (name string, params []string, variadic bool, doc string, ok bool) {
	ast.Inspect(s.tree(file), func(n ast.Node) bool {
		if ok {
			return false
		}
		switch n := n.(type) {
		case ast.Fn:
			if n.NamePos == pos {
				name, params, variadic, ok = n.Name, withDots(n.Args, n.IsVariadic), n.IsVariadic, true
			}
		case ast.ClassDefinition:
			if n.NamePos == pos {
				name, params, ok = n.Name, n.Args, true
			}
		case ast.Decl:
			if fn, isFn := n.Value.(ast.Fn); isFn && n.Pos == pos {
				name, params, variadic, ok = n.Name, withDots(fn.Args, fn.IsVariadic), fn.IsVariadic, true
			}
		}
		return true
	})
	return name, params, variadic, "", ok
}

// withDots writes a variadic function's last parameter as ...name
func withDots(args []string, variadic bool) []string {
	if !variadic || len(args) == 0 {
		return args
	}
	out := slices.Clone(args)
	out[len(out)-1] = "..." + out[len(out)-1]
	return out
}

// parseSignature reads a host function's documented signature, as in
// "readFile(path)" or "format(t, layout?)"
func parseSignature(sig string) (name string, params []string, variadic bool) {
	open := strings.IndexByte(sig, '(')
	if open < 0 || !strings.HasSuffix(sig, ")") {
		return sig, nil, false
	}
	name = sig[:open]
	inner := strings.TrimSpace(sig[open+1 : len(sig)-1])
	if inner == "" {
		return name, nil, false
	}
	params = strings.Split(inner, ", ")
	return name, params, strings.HasPrefix(params[len(params)-1], "...")
}

// parserPackage is the package a file of the analysis declares
func parserPackage(s *snapshot, file string) string {
	pkg := s.packageOf(file)
	return pkg[strings.IndexByte(pkg, 0)+1:]
}
