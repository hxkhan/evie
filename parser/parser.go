package parser

import (
	"bytes"
	"fmt"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/internal/lexer"
	"github.com/hxkhan/evie/internal/token"
)

const maxParseDepth = 256

// Error is a syntax error: a report with the offending span and, where it
// helps, the construct that was left open
type Error struct {
	diag.Report
}

// Error returns the report prefixed with "syntax error: "
func (e *Error) Error() string {
	return "syntax error: " + e.Report.Error()
}

// ErrorList is what parsing returns when a source has more than one syntax
// error, in the order they appear. errors.As finds each *Error in it.
type ErrorList []*Error

// Error returns the first error, and how many more there are
func (l ErrorList) Error() string {
	if len(l) == 1 {
		return l[0].Error()
	}
	return fmt.Sprintf("%s (and %d more)", l[0].Error(), len(l)-1)
}

// Unwrap returns every error in the list, for errors.Is and errors.As
func (l ErrorList) Unwrap() []error {
	errs := make([]error, len(l))
	for i, e := range l {
		errs[i] = e
	}
	return errs
}

// Diagnostics returns the report of every error, for tools that show them all
func (l ErrorList) Diagnostics() []*diag.Report {
	reports := make([]*diag.Report, len(l))
	for i, e := range l {
		reports[i] = &e.Report
	}
	return reports
}

// maxErrors is how many syntax errors one source reports before parsing stops
const maxErrors = 10

// stopParsing is raised when maxErrors is reached
type stopParsing struct{}

type parser struct {
	*lexer.Lexer
	src   *diag.Source
	last  token.Token
	depth int
	errs  []*Error
	text  []byte // the parser's private copy of the source
}

// openBefore returns the brackets ( [ { and template backticks still open
// before pos, innermost last. Parsing does not keep track of them, so that
// correct code pays nothing; after an error the source is lexed again up to
// where the parser stopped.
func (ps *parser) openBefore(pos diag.Pos) []token.Token {
	var open []token.Token
	lex := lexer.New(ps.text)
	for {
		tok := lex.NextToken()
		if tok.IsEOS() || tok.Pos.Line > pos.Line || tok.Pos.Line == pos.Line && tok.Pos.Column >= pos.Column {
			return open
		}
		open = track(open, tok)
	}
}

// track updates a stack of open brackets with the next token
func track(open []token.Token, tok token.Token) []token.Token {
	if tok.Type != token.Simple {
		return open
	}
	top := ""
	if len(open) > 0 {
		top = open[len(open)-1].Literal
	}
	switch tok.Literal {
	case "(", "[", "{":
		return append(open, tok)
	case ")", "]", "}":
		if opener(tok.Literal) == top {
			return open[:len(open)-1]
		}
	case "`":
		// a backtick closes the template it is in, or opens one
		if top == "`" {
			return open[:len(open)-1]
		}
		return append(open, tok)
	}
	return open
}

// opener is the bracket a closing one pairs with, or "" for any other token
func opener(closer string) string {
	switch closer {
	case ")":
		return "("
	case "]":
		return "["
	case "}":
		return "{"
	}
	return ""
}

// opens reports whether a closing bracket matches one of the open ones
func opens(open []token.Token, closer token.Token) bool {
	return slices.ContainsFunc(open, func(t token.Token) bool { return t.Literal == opener(closer.Literal) })
}

// isClosing reports whether tok is ) ] or }
func isClosing(tok token.Token) bool {
	return tok.Type == token.Simple && opener(tok.Literal) != ""
}

// startsStatement holds the words that begin a statement and cannot appear
// inside parentheses or brackets
var startsStatement = []string{"var", "const", "class", "if", "while", "for", "return", "switch", "break", "continue", "echo"}

// statement parses one statement. A syntax error in it does not end parsing:
// the error is recorded, the rest of the statement is skipped, and parsing
// goes on, so one mistake does not hide the others. inBlock says a '}' ends
// the enclosing block, so skipping stops there.
func (ps *parser) statement(inBlock bool, parse func()) {
	start := ps.PeekToken().Pos
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		e, ok := r.(*Error)
		if !ok {
			panic(r)
		}
		ps.record(e)
		// the brackets the broken statement opened and did not close
		var open []token.Token
		if before, now := ps.openBefore(start), ps.openBefore(ps.PeekToken().Pos); len(now) > len(before) {
			open = now[len(before):]
		}
		ps.skip(inBlock, open, e.Pos)
		// a statement that failed at its first token without taking it must not
		// be tried again from the same place
		if next := ps.PeekToken(); next.Pos == start && !next.IsEOS() && !(inBlock && next.IsSimple("}")) {
			ps.NextToken()
			ps.skip(inBlock, nil, diag.Pos{})
		}
	}()
	parse()
}

// record keeps a syntax error, once, and stops parsing when there are too many
func (ps *parser) record(e *Error) {
	if n := len(ps.errs); n > 0 && ps.errs[n-1].Pos == e.Pos {
		return // the same place again adds nothing
	}
	ps.errs = append(ps.errs, e)
	if len(ps.errs) >= maxErrors {
		panic(stopParsing{})
	}
}

// skip passes over the rest of a broken statement whose open brackets are
// still open: up to a ';' or a new line once they are closed, or up to the '}'
// of the enclosing block. Some signs say a '(' or '[' was never closed, and
// skipping stops there too: the error is at the start of a new line, a new
// statement starts a line, or a bracket closes that the statement never opened.
func (ps *parser) skip(inBlock bool, open []token.Token, at diag.Pos) {
	for first := true; ; first = false {
		next := ps.PeekToken()
		if next.IsEOS() {
			return
		}
		if isClosing(next) && !opens(open, next) {
			open = nil // it closes something outside this statement
		}
		unclosed := false
		if n := len(open); n > 0 {
			top := open[n-1].Literal
			unclosed = (top == "(" || top == "[") && next.NewlineBefore &&
				(first && next.Pos == at || next.Type == token.Word && slices.Contains(startsStatement, next.Literal))
		}
		if unclosed {
			return
		}
		if len(open) == 0 {
			switch {
			case next.IsSimple(";"):
				ps.NextToken()
				return
			case next.IsSimple("}") && inBlock, next.NewlineBefore:
				return
			}
		}
		ps.NextToken()
		open = track(open, next)
	}
}

// failed is the error to return once parsing is over: nil, the one error, or
// all of them
func (ps *parser) failed() error {
	switch len(ps.errs) {
	case 0:
		return nil
	case 1:
		return ps.errs[0]
	}
	return ErrorList(ps.errs)
}

var keywords = []string{
	"package", "imports",
	"nil", "true", "false",
	"var", "const", "fn", "class", "super",
	"echo",
	"return",
	"if", "else",
	"switch", "case", "default",
	"is",
	"await", "go",
	"catch", "throw",
	"while", "break", "continue",
	"for", "in",
}

var operators = map[string]ast.Operator{
	"+": ast.AddOp, "-": ast.SubOp, "*": ast.MulOp, "/": ast.DivOp, "%": ast.ModOp,
	"+=": ast.AddOp, "-=": ast.SubOp, "*=": ast.MulOp, "/=": ast.DivOp, "%=": ast.ModOp,
	"==": ast.EqOp, "!=": ast.NotEqOp, "<": ast.LtOp, ">": ast.GtOp, "<=": ast.LtEqOp, ">=": ast.GtEqOp,
	"||": ast.OrOp, "&&": ast.AndOp,
}

var precedence = map[string]int{
	"||": 0,
	"&&": 1,
	"<":  2, ">": 2, "==": 2, "!=": 2, "<=": 2, ">=": 2,
	"+": 3, "-": 3,
	"*": 4, "/": 4, "%": 4,
}

const (
	isPrec    = 2
	unaryPrec = 5
)

// PeekPackageName scans only the leading tokens of src to read the declared
// package name without parsing the rest of the file. It returns "" for bare
// scripts or when the header does not start with a valid `package <name>`.
func PeekPackageName(src *diag.Source) string {
	lex := lexer.New(src.Text)
	first := lex.NextToken()
	if !first.IsWord("package") {
		return ""
	}
	name := lex.NextToken()
	if name.Type != token.Word || slices.Contains(keywords, name.Literal) {
		return ""
	}
	return strings.Clone(name.Literal) // not a view into the caller's buffer
}

// Parse parses a script whose source has no name
func Parse(input []byte) (ast.Node, error) {
	return ParseSource(diag.NewSource("", input))
}

// ParseSource parses a script; errors name the source and show its text.
// Parsing goes on after a syntax error, so a source with several mistakes
// reports them all (up to ten) as an ErrorList; a single one is an *Error.
// With errors, the returned node holds the statements that did parse.
func ParseSource(src *diag.Source) (node ast.Node, err error) {
	// the lexer hands out strings that share memory with its input, and they end up
	// in the AST and in compiled constants; lex a private copy, so a caller reusing
	// its buffer cannot change names or strings of the parsed program
	text := bytes.Clone(src.Text)
	ps := parser{Lexer: lexer.New(text), src: src, text: text}

	defer func() {
		if r := recover(); r != nil {
			switch e := r.(type) {
			case stopParsing:
				// too many errors; report the ones found
			case *Error:
				ps.errs = append(ps.errs, e)
			case runtime.Error:
				// a bug in the parser or lexer; report it instead of taking the host down
				err = fmt.Errorf("internal parser error: %v", e)
				return
			case error:
				err = fmt.Errorf("ParseError: %s", e.Error())
				return
			default:
				ps.errs = append(ps.errs, ps.report(ps.last.Pos, fmt.Sprint(r)))
			}
		}
		if failed := ps.failed(); failed != nil {
			err = failed
		}
	}()

	for ps.consume(";") {
	}
	if ps.PeekToken().IsEOS() {
		return nil, ps.report(ps.PeekToken().Pos, "the source is empty")
	}

	if ps.PeekToken().IsWord("package") {
		pack := &ast.Package{}
		ps.statement(false, func() {
			ps.consume("package")
			*pack = ps.parsePackage()
		})
		for {
			for ps.consume(";") {
			}
			if ps.PeekToken().IsEOS() {
				return *pack, nil
			}
			ps.statement(false, func() { pack.Code = append(pack.Code, ps.parse(0, false)) })
		}
	}

	var cb ast.Block
	for {
		for ps.consume(";") {
		}
		if ps.PeekToken().IsEOS() {
			return cb, nil
		}
		ps.statement(false, func() { cb.Code = append(cb.Code, ps.parse(0, false)) })
	}
}

// report builds a syntax error at a position
func (ps *parser) report(pos diag.Pos, msg string, notes ...diag.Note) *Error {
	return &Error{Report: diag.Report{Message: msg, Source: ps.src, Pos: pos, Notes: notes}}
}

// fail stops parsing with an error at a position
func (ps *parser) fail(pos diag.Pos, msg string, notes ...diag.Note) {
	panic(ps.report(pos, msg, notes...))
}

// expected stops parsing because the next token is not what the grammar needs
// here; opened, when known, points at the construct being parsed
func (ps *parser) expected(what string, opened token.Token, context string) {
	next := ps.PeekToken()
	msg := fmt.Sprintf("expected %s, found %s", what, next.Describe())
	if next.IsEOS() {
		msg = fmt.Sprintf("expected %s, but the source ends here", what)
	}
	if opened.Pos.Known() && context != "" {
		ps.fail(next.Pos, msg, diag.Note{Source: ps.src, Pos: opened.Pos, Text: context})
	}
	ps.fail(next.Pos, msg)
}

// consume will either try to consume a simple or a word
func (ps *parser) consume(lit string) bool {
	if next := ps.PeekToken(); next.IsSimple(lit) || next.IsWord(lit) {
		ps.last = ps.NextToken()
		return true
	}
	return false
}

func (ps *parser) consumeName(lit string) bool {
	if next := ps.PeekToken(); next.IsWord(lit) && !slices.Contains(keywords, lit) {
		ps.last = ps.NextToken()
		return true
	}
	return false
}

// name consumes an identifier or fails saying what it was needed for
func (ps *parser) name(purpose string) token.Token {
	next := ps.PeekToken()
	if next.Type != token.Word {
		ps.expected(purpose, token.Token{}, "")
	}
	if slices.Contains(keywords, next.Literal) {
		ps.fail(next.Pos, fmt.Sprintf("'%s' is a keyword and cannot be used as %s", next.Literal, purpose))
	}
	ps.last = ps.NextToken()
	return ps.last
}

func (ps *parser) parsePackage() ast.Package {
	keyword := ps.last
	name := ps.name("the package name")
	pack := ast.Package{Pos: keyword.Pos.To(name.Pos), Name: name.Literal, NamePos: name.Pos}
	if ps.consume("imports") {
		imports := ps.last
		if !ps.consume("(") {
			ps.expected("'(' after imports", imports, "")
		}
		open := ps.last
		if !ps.consume(")") {
			pack.Imports = ps.parseImportList(open)
		}
	}
	return pack
}

// parseImportList parses a comma-separated list of `"path"` or `"path" as alias` entries.
func (ps *parser) parseImportList(open token.Token) []ast.ImportSpec {
	var imports []ast.ImportSpec
	ps.list(open, ")", "the import list", func() {
		next := ps.PeekToken()
		if next.Type != token.String {
			ps.expected("a package name in quotes", open, "in this import list")
		}
		ps.NextToken()
		textPos := next.Pos
		textPos.Column++ // text starts after the opening quote
		spec := ast.ImportSpec{
			Pos:  next.Pos,
			Path: ps.unescape(next.Literal, textPos, ""),
		}
		if ps.consume("as") {
			alias := ps.name("an alias name after 'as'")
			spec.Alias = alias.Literal
			spec.AliasPos = alias.Pos
			spec.Pos = next.Pos.To(alias.Pos)
		}
		imports = append(imports, spec)
	})
	return imports
}

// list parses the comma-separated items of a list whose opening token was just
// consumed, up to and including close; a trailing comma is allowed. what names
// the list in errors, as in "this '(' opens the argument list".
func (ps *parser) list(open token.Token, close, what string, item func()) {
	if ps.consume(close) {
		return
	}
	for {
		item()
		if ps.consume(close) {
			return
		}
		if !ps.consume(",") {
			ps.expected(fmt.Sprintf("',' or '%s'", close), open, fmt.Sprintf("this '%s' opens %s", open.Literal, what))
		}
		if ps.consume(close) {
			return
		}
	}
}

func (ps *parser) parseFloat(tok token.Token) float64 {
	// numbers are decimal digits with an optional fraction; strconv alone would
	// also take exponents, hex floats and the like
	valid := tok.Literal != ""
	for _, c := range tok.Literal {
		if (c < '0' || c > '9') && c != '.' {
			valid = false
		}
	}
	num, err := strconv.ParseFloat(tok.Literal, 64)
	if err != nil || !valid {
		ps.fail(tok.Pos, fmt.Sprintf("'%s' is not a valid number", tok.Literal))
	}
	return num
}

func (ps *parser) handleWords(main token.Token, asExpr bool, precedenceLevel int) ast.Node {
	switch main.Literal {
	case "echo":
		return ast.Echo{Pos: main.Pos, Value: ps.parse(0, true)}
	case "fn":
		return ps.parseFn(main, asExpr, precedenceLevel)
	case "class":
		return ps.parseClass(main)
	case "super":
		if !ps.PeekToken().IsSimple(".") {
			ps.expected("'.' and a method name after 'super'", main, "")
		}
		left := ps.parseInfixExpression(ast.Ident{Pos: main.Pos, Name: "super"}, precedenceLevel)
		if ps.consume("=") {
			value := ps.parse(0, true)
			return ast.Assign{Pos: left.Where().To(value.Where()), Lhs: left, Value: value}
		}
		return left
	case "go":
		call := ps.parse(unaryPrec, true)
		return ps.parseInfixExpression(ast.Go{Pos: main.Pos.To(call.Where()), Fn: call}, precedenceLevel)
	case "await":
		return ps.parseAwait(main, precedenceLevel)
	case "nil":
		return ps.parseInfixExpression(ast.Input[struct{}]{Pos: main.Pos}, precedenceLevel)
	case "true":
		return ps.parseInfixExpression(ast.Input[bool]{Pos: main.Pos, Value: true}, precedenceLevel)
	case "false":
		return ps.parseInfixExpression(ast.Input[bool]{Pos: main.Pos, Value: false}, precedenceLevel)
	case "if":
		return ps.parseConditional(main)
	case "while":
		return ps.parseWhile(main)
	case "for":
		return ps.parseFor(main)
	case "switch":
		return ps.parseSwitch(main)
	case "break":
		return ast.Break{Pos: main.Pos}
	case "continue":
		return ast.Continue{Pos: main.Pos}
	case "return":
		ret := ast.Return{Pos: main.Pos}
		if ps.PeekToken().Line == main.Line && !ps.PeekToken().IsSimple("}") && !ps.PeekToken().IsSimple(";") && !ps.PeekToken().IsEOS() {
			ret.Value = ps.parse(0, true)
		} else {
			ret.Value = ast.Input[struct{}]{Pos: main.Pos}
		}
		return ret

	case "catch":
		var node ast.Node
		if ps.consume("{") {
			open := ps.last
			node = ast.Catch{Pos: main.Pos, Action: ps.parseBlock(open)}
		} else if ps.consume("(") {
			open := ps.last
			e := ps.parse(0, true)
			if !ps.consume(")") {
				ps.expected("')'", open, "this '(' opens the catch")
			}
			node = ast.Catch{Pos: main.Pos.To(ps.last.Pos), Action: e}
		} else {
			ps.expected("'(' or '{' after catch", main, "")
		}
		return ps.parseInfixExpression(node, precedenceLevel)

	case "throw":
		th := ast.Throw{Pos: main.Pos}
		if ps.PeekToken().Line == main.Line && !ps.PeekToken().IsSimple("}") && !ps.PeekToken().IsSimple(";") && !ps.PeekToken().IsEOS() {
			th.Value = ps.parse(0, true)
		} else {
			th.Value = ast.Input[string]{Pos: main.Pos, Value: "error"}
		}
		return th

	case "var":
		name := ps.name("a name after var")
		if !ps.consume(":=") {
			ps.expected(fmt.Sprintf("':=' after 'var %s'", name.Literal), main, "")
		}
		return ast.Decl{Pos: name.Pos, Name: name.Literal, IsStatic: false, Value: ps.parse(0, true)}

	case "const":
		// `const x := v` says explicitly what `x := v` already means
		name := ps.name("a name after const")
		if !ps.consume(":=") {
			ps.expected(fmt.Sprintf("':=' after 'const %s'", name.Literal), main, "")
		}
		return ast.Decl{Pos: name.Pos, Name: name.Literal, IsStatic: true, Value: ps.parse(0, true)}

	default:
		return ps.parseIdent(main, precedenceLevel)
	}
}

func (ps *parser) parseStringTemplate(main token.Token) (st ast.StringTemplate) {
	var part strings.Builder // the literal text since the last interpolation
	st.Pos = main.Pos

	for {
		if ps.consume("`") {
			break
		} else if ps.consume("{") {
			lb := ps.last
			st.Parts = append(st.Parts, part.String())
			part.Reset()
			st.Args = append(st.Args, ps.parse(0, true))
			if !ps.consume("}") {
				ps.expected("'}'", lb, "this '{' opens an interpolation")
			}
		} else if ps.PeekToken().Type == token.String {
			text := ps.NextToken()
			part.WriteString(ps.unescape(text.Literal, text.Pos, "`{}"))
		} else {
			ps.expected("the closing '`'", main, "this template string is unterminated")
		}
	}

	st.Pos = main.Pos.To(ps.last.Pos)
	st.Parts = append(st.Parts, part.String())
	return st
}

func (ps *parser) parseAwait(main token.Token, precedenceLevel int) ast.Node {
	// await.all(x, y, z), await.any(x, y, z), await.race(x, y, z), or await.timeout(t, ms)
	if !ps.consume(".") {
		task := ps.parse(unaryPrec, true)
		return ps.parseInfixExpression(ast.Await{Pos: main.Pos.To(task.Where()), Task: task}, precedenceLevel)
	}

	var node ast.Node
	if ps.consumeName("all") {
		node = ast.AwaitAll{Pos: main.Pos.To(ps.last.Pos), Tasks: ps.parseArgsList()}
	} else if ps.consumeName("any") {
		node = ast.AwaitAny{Pos: main.Pos.To(ps.last.Pos), Tasks: ps.parseArgsList()}
	} else if ps.consumeName("race") {
		node = ast.AwaitRace{Pos: main.Pos.To(ps.last.Pos), Tasks: ps.parseArgsList()}
	} else if ps.consumeName("timeout") {
		args := ps.parseArgsList()
		if len(args) != 2 {
			ps.fail(main.Pos.To(ps.last.Pos), fmt.Sprintf("await.timeout takes 2 arguments (task, ms) but %d were supplied", len(args)))
		}
		node = ast.AwaitTimeout{Pos: main.Pos.To(ps.last.Pos), Task: args[0], Timeout: args[1]}
	} else {
		ps.expected("'all', 'any', 'race' or 'timeout' after 'await.'", main, "")
		return nil
	}
	return ps.parseInfixExpression(node, precedenceLevel)
}

func (ps *parser) parseIdent(main token.Token, precedenceLevel int) ast.Node {
	if slices.Contains(keywords, main.Literal) {
		if ps.PeekToken().IsSimple(":=") {
			ps.fail(main.Pos, fmt.Sprintf("'%s' is a keyword and cannot be used as a variable name", main.Literal))
		}
		ps.fail(main.Pos, fmt.Sprintf("unexpected '%s'", main.Literal))
	}

	// handle const declarations explicitly
	if ps.consume(":=") {
		return ast.Decl{Pos: main.Pos, Name: main.Literal, IsStatic: true, Value: ps.parse(0, true)}
	}

	// try infix stuff
	left := ps.parseInfixExpression(ast.Ident{Pos: main.Pos, Name: main.Literal}, precedenceLevel)

	if ps.consume("=") {
		value := ps.parse(0, true)
		return ast.Assign{Pos: left.Where().To(value.Where()), Lhs: left, Value: value}
	}
	if ps.consume("+=") || ps.consume("-=") || ps.consume("*=") || ps.consume("/=") || ps.consume("%=") {
		// read the operator before parsing the rhs; the evaluation order of map indexing vs calls in a composite literal is unspecified
		operator := operators[ps.last.Literal]
		rhs := ps.parse(0, true)
		return ast.MutableBinOp{Pos: left.Where().To(rhs.Where()), Operator: operator, Lhs: left, Rhs: rhs}
	}

	return left
}

func (ps *parser) parseConditional(main token.Token) ast.Node {
	node := ast.Conditional{Pos: main.Pos}
	node.Condition = ps.parse(0, true)
	if !ps.consume("{") {
		ps.expected("'{' after the condition", main, "in this if")
	}
	node.Action = ps.parseBlock(ps.last)
	if ps.consume("else") {
		elseTok := ps.last
		if ps.consume("if") {
			// an else-if nests the rest of the chain, and every later pass recurses
			// into it, so it counts towards the nesting limit like any other nesting
			ps.depth++
			if ps.depth > maxParseDepth {
				ps.fail(ps.last.Pos, "this else-if chain is nested too deeply; use a switch")
			}
			node.Otherwise = ps.parseConditional(ps.last)
			ps.depth--
		} else {
			if !ps.consume("{") {
				ps.expected("'{' or 'if' after else", elseTok, "")
			}
			node.Otherwise = ps.parseBlock(ps.last)
		}
	}
	return node
}

func (ps *parser) parseWhile(main token.Token) ast.Node {
	node := ast.While{Pos: main.Pos}
	node.Condition = ps.parse(0, true)
	if !ps.consume("{") {
		ps.expected("'{' after the condition", main, "in this while")
	}
	node.Action = ps.parseBlock(ps.last)
	return node
}

func (ps *parser) parseFor(main token.Token) ast.Node {
	hasParen := ps.consume("(")
	first := ps.name("the loop variable")

	var indexVar, itemVar string
	var indexPos, itemPos diag.Pos
	if ps.consume(",") {
		indexVar, indexPos = first.Literal, first.Pos
		item := ps.name("the item variable after ','")
		itemVar, itemPos = item.Literal, item.Pos
	} else {
		itemVar, itemPos = first.Literal, first.Pos
	}

	varsClosedParen := false
	if hasParen && ps.consume(")") {
		varsClosedParen = true
	}

	if !ps.consume("in") {
		ps.expected("'in'", main, "in this for loop")
	}

	col := ps.parse(0, true)
	if hasParen && !varsClosedParen {
		if !ps.consume(")") {
			ps.expected("')'", main, "in this for loop")
		}
	}

	if !ps.consume("{") {
		ps.expected("'{' after the collection", main, "in this for loop")
	}
	body := ps.parseBlock(ps.last)

	return ast.For{
		Pos:        main.Pos,
		IndexVar:   indexVar,
		IndexPos:   indexPos,
		ItemVar:    itemVar,
		ItemPos:    itemPos,
		Collection: col,
		Action:     body,
	}
}

func (ps *parser) parseSwitch(main token.Token) ast.Node {
	sw := ast.Switch{Pos: main.Pos}

	// Check if this is a value switch or a tagless switch
	// If the next token is '{', it's a tagless switch (switch { ... })
	if !ps.consume("{") {
		sw.Value = ps.parse(0, true)
		if !ps.consume("{") {
			ps.expected("'{'", main, "in this switch")
		}
	}
	open := ps.last

	caseBody := func() ast.Block {
		var block ast.Block
		for {
			ps.consume(";")
			next := ps.PeekToken()
			if next.IsWord("case") || next.IsWord("default") || next.IsSimple("}") {
				break
			}
			if next.IsEOS() {
				ps.expected("'}'", open, "this '{' opens the switch")
			}
			ps.statement(true, func() { block.Code = append(block.Code, ps.parseNode(0, false)) })
			ps.consume(";")
		}
		return block
	}

	for !ps.consume("}") {
		tok := ps.PeekToken()
		if tok.IsWord("case") {
			ps.NextToken() // consume 'case'
			var exprs []ast.Node
			for {
				exprs = append(exprs, ps.parse(0, true))
				if !ps.consume(",") {
					break
				}
			}
			if !ps.consume(":") && !ps.consume("=>") {
				ps.expected("':' after the case values", tok, "")
			}
			sw.Cases = append(sw.Cases, ast.SwitchCase{Pos: tok.Pos, Values: exprs, Action: caseBody()})
		} else if tok.IsWord("default") {
			ps.NextToken() // consume 'default'
			if sw.Default != nil {
				ps.fail(tok.Pos, "a switch can have only one default case")
			}
			if !ps.consume(":") && !ps.consume("=>") {
				ps.expected("':' after default", tok, "")
			}
			sw.Default = caseBody()
		} else {
			ps.expected("'case', 'default' or '}'", open, "this '{' opens the switch")
		}
	}
	sw.End = ps.last.Pos

	return sw
}

// parseBlock parses statements up to the '}' that closes open
func (ps *parser) parseBlock(open token.Token) ast.Node {
	block := ast.Block{Pos: open.Pos}
	for {
		for ps.consume(";") {
		}
		if ps.consume("}") {
			block.End = ps.last.Pos
			break
		}
		if ps.PeekToken().IsEOS() {
			ps.expected("'}'", open, "this '{' is never closed")
		}
		ps.statement(true, func() { block.Code = append(block.Code, ps.parseNode(0, false)) })
	}
	return block
}

// helper to parse an fn
func (ps *parser) parseFn(main token.Token, asExpr bool, precedenceLevel int) ast.Node {
	fn := ast.Fn{Pos: main.Pos, UsedAsExpr: asExpr}
	if ps.PeekToken().Type == token.Word {
		name := ps.name("the function name")
		fn.Name = name.Literal
		fn.NamePos = name.Pos
		fn.Pos = main.Pos.To(name.Pos)
	}
	fn.Args, fn.ArgPos, fn.IsVariadic = ps.parseNamesList(main, "the parameter list")

	if ps.consume("{") {
		fn.Action = ps.parseBlock(ps.last)
		if asExpr {
			return ps.parseInfixExpression(fn, precedenceLevel)
		}
	} else if ps.consume("=>") {
		// an arrow body may also be an assignment, as in `fn reset() => this.n = 0`
		body := ps.parseNode(0, true)
		ps.mustBeValue(body, true)
		fn.Action = ast.Return{Pos: body.Where(), Value: body}
	} else {
		ps.expected("'{' or '=>' after the parameters", main, "in this function")
	}
	return fn
}

// helper to parse a class
func (ps *parser) parseClass(main token.Token) ast.Node {
	cls := ast.ClassDefinition{Pos: main.Pos}
	if ps.PeekToken().Type == token.Word {
		name := ps.name("the class name")
		cls.Name = name.Literal
		cls.NamePos = name.Pos
		cls.Pos = main.Pos.To(name.Pos)
	}
	var isVariadic bool
	cls.Args, cls.ArgPos, isVariadic = ps.parseNamesList(main, "the field list")
	if isVariadic {
		ps.fail(ps.last.Pos, "class fields cannot be variadic")
	}

	if ps.consume(":") {
		parent := ps.name("the parent class after ':'")
		cls.Parent, cls.ParentPos = parent.Literal, parent.Pos
	}

	if !ps.consume("{") {
		ps.expected("'{' for the class body", main, "in this class")
	}
	cls.Action = ps.parseBlock(ps.last)

	return cls
}

// helper to parse names surrounded by parentheses
func (ps *parser) parseNamesList(main token.Token, what string) ([]string, []diag.Pos, bool) {
	if !ps.consume("(") {
		ps.expected("'(' to open "+what, main, "")
	}
	open := ps.last

	var args []string
	var positions []diag.Pos
	isVariadic := false
	ps.list(open, ")", what, func() {
		if ps.consume("...") {
			isVariadic = true
			name := ps.name("a parameter name after '...'")
			args, positions = append(args, name.Literal), append(positions, name.Pos)
			if !ps.PeekToken().IsSimple(")") {
				ps.expected("')' after the variadic parameter", open, "this '(' opens "+what)
			}
			return
		}
		name := ps.name("a name")
		args, positions = append(args, name.Literal), append(positions, name.Pos)
	})
	return args, positions, isVariadic
}

func (ps *parser) parseArgsList() []ast.Node {
	if !ps.consume("(") {
		ps.expected("'('", ps.last, "")
	}
	return ps.parseArgs(ps.last, "the argument list")
}

// parseArgs parses call arguments after the '(' open
func (ps *parser) parseArgs(open token.Token, what string) []ast.Node {
	var args []ast.Node
	ps.list(open, ")", what, func() {
		args = append(args, ps.parse(0, true))
	})
	return args
}

func (ps *parser) parseObject(main token.Token) ast.Node {
	obj := ast.Object{Pos: main.Pos}
	ps.list(main, "}", "the object", func() {
		// a key may be named like a keyword, as a member after '.' may
		if ps.PeekToken().Type != token.Word {
			ps.expected("a field name", token.Token{}, "")
		}
		key := ps.NextToken()
		ps.last = key
		if !ps.consume(":") {
			ps.expected(fmt.Sprintf("':' after the field name '%s'", key.Literal), main, "in this object")
		}
		obj.Fields = append(obj.Fields, ast.ObjectField{Key: key.Literal, KeyPos: key.Pos, Value: ps.parse(0, true)})
	})
	obj.Pos = main.Pos.To(ps.last.Pos)
	obj.End = ps.last.Pos
	return obj
}

func (ps *parser) parseArrayLiteral(main token.Token) ast.Node {
	arr := ast.ArrayLiteral{Pos: main.Pos}
	ps.list(main, "]", "the array", func() {
		arr.Elements = append(arr.Elements, ps.parse(0, true))
	})
	arr.Pos = main.Pos.To(ps.last.Pos)
	arr.End = ps.last.Pos
	return arr
}

// parse parses a statement, or a value when asExpr is set; a statement where
// a value is needed is a syntax error instead of a silent nil
func (ps *parser) parse(precedenceLevel int, asExpr bool) ast.Node {
	node := ps.parseNode(precedenceLevel, asExpr)
	if asExpr {
		ps.mustBeValue(node, false)
	}
	return node
}

// mustBeValue fails when node is a statement, which has no value to use;
// assignments pass when assignOK is set
func (ps *parser) mustBeValue(node ast.Node, assignOK bool) {
	var what, hint string
	switch node := node.(type) {
	case ast.Decl:
		what, hint = "a declaration", fmt.Sprintf("declare '%s' on a line of its own, then use it", node.Name)
	case ast.Assign, ast.MutableBinOp:
		if assignOK {
			return
		}
		what, hint = "an assignment", "assign on a line of its own, then use the variable"
	case ast.Return:
		what = "'return'"
	case ast.Break:
		what = "'break'"
	case ast.Continue:
		what = "'continue'"
	case ast.Echo:
		if assignOK {
			return
		}
		what = "'echo'"
	case ast.Conditional:
		what = "an 'if' statement"
	case ast.Switch:
		what = "a 'switch' statement"
	case ast.While, ast.For:
		what = "a loop"
	case ast.ClassDefinition:
		what = "a class definition"
	default:
		return
	}
	err := ps.report(node.Where(), "expected a value, found "+what)
	err.Label = "this has no value"
	err.Hint = hint
	panic(err)
}

func (ps *parser) parseNode(precedenceLevel int, asExpr bool) (node ast.Node) {
	ps.depth++
	if ps.depth > maxParseDepth {
		ps.fail(ps.PeekToken().Pos, "expression is nested too deeply")
	}
	defer func() { ps.depth-- }()

	// handle parentheses explicitly
	if ps.consume("(") {
		open := ps.last
		// reset precedence to 0 inside parentheses
		expr := ps.parse(0, true)
		// ensure the closing parenthesis is present
		if !ps.consume(")") {
			ps.expected("')'", open, "this '(' is never closed")
		}
		// the sub-expression within the parentheses becomes the new left
		return ps.parseInfixExpression(expr, precedenceLevel)
	}

	main := ps.PeekToken()
	if main.IsEOS() {
		ps.fail(main.Pos, "unexpected end of source")
	}

	switch {
	case main.Type == token.String:
		tok := ps.NextToken()
		text := tok.Pos
		text.Column++ // the text starts after the opening quote
		node = ast.Input[string]{Pos: tok.Pos, Value: ps.unescape(tok.Literal, text, "")}
	case main.Type == token.Number:
		node = ast.Input[float64]{Pos: main.Pos, Value: ps.parseFloat(ps.NextToken())}

	case main.Type == token.Word:
		return ps.handleWords(ps.NextToken(), asExpr, precedenceLevel)

	case main.IsSimple("{"):
		node = ps.parseObject(ps.NextToken())

	case main.IsSimple("["):
		node = ps.parseArrayLiteral(ps.NextToken())

	case main.IsSimple("+"):
		ps.NextToken()
		if ps.PeekToken().Type == token.Number {
			num := ps.NextToken()
			node = ast.Input[float64]{Pos: main.Pos.To(num.Pos), Value: ps.parseFloat(num)}
		} else {
			operand := ps.parse(unaryPrec, true)
			node = ast.Plus{Pos: main.Pos.To(operand.Where()), Value: operand}
		}

	case main.IsSimple("-"):
		ps.NextToken()
		if ps.PeekToken().Type == token.Number {
			num := ps.NextToken()
			node = ast.Input[float64]{Pos: main.Pos.To(num.Pos), Value: -ps.parseFloat(num)}
		} else {
			operand := ps.parse(unaryPrec, true)
			node = ast.Neg{Pos: main.Pos.To(operand.Where()), Value: operand}
		}

	case main.IsSimple("!"):
		ps.NextToken()
		operand := ps.parse(unaryPrec, true)
		node = ast.Not{Pos: main.Pos.To(operand.Where()), Value: operand}

	case main.IsSimple("`"):
		node = ps.parseStringTemplate(ps.NextToken())
	case main.Type == token.Invalid:
		if strings.HasPrefix(main.Literal, "\"") || strings.HasPrefix(main.Literal, "`") {
			ps.fail(main.Pos, "this string is never closed")
		}
		if main.Literal == "/*" {
			ps.fail(main.Pos, "this block comment is never closed")
		}
		ps.fail(main.Pos, fmt.Sprintf("invalid token '%s'", main.Literal))
	default:
		if main.IsSimple(")") || main.IsSimple("]") || main.IsSimple("}") {
			if open := ps.openBefore(main.Pos); len(open) > 0 {
				// closing what is open, as in `f(1, )` or `{1 +}`, leaves out a value
				top := open[len(open)-1]
				if top.Literal == opener(main.Literal) {
					ps.fail(main.Pos, fmt.Sprintf("expected a value, found '%s'", main.Literal))
				}
				// closing something further out leaves the innermost one open
				if opens(open, main) {
					ps.fail(main.Pos, fmt.Sprintf("unexpected '%s'", main.Literal),
						diag.Note{Source: ps.src, Pos: top.Pos, Text: fmt.Sprintf("this '%s' is never closed", top.Literal)})
				}
			}
			ps.fail(main.Pos, fmt.Sprintf("unexpected '%s': nothing is open here", main.Literal))
		}
		ps.fail(main.Pos, fmt.Sprintf("unexpected '%s'", main.Literal))
	}

	// parse the left-hand side
	return ps.parseInfixExpression(node, precedenceLevel)
}

func (ps *parser) parseInfixExpression(left ast.Node, precedenceLevel int) ast.Node {
	for {
		next := ps.PeekToken()

		// a '(' or '[' that starts a line starts a new statement; it neither calls
		// nor indexes the expression on the line before
		if next.NewlineBefore && (next.IsSimple("(") || next.IsSimple("[")) {
			break
		}

		// field access; a member may be named like a keyword, as in task.await()
		if next.IsSimple(".") {
			ps.NextToken() // consume '.'
			if ps.PeekToken().Type != token.Word {
				ps.expected("a member name after '.'", token.Token{}, "")
			}
			name := ps.NextToken()
			left = ast.FieldAccess{Pos: left.Where().To(name.Pos), Lhs: left, Rhs: name.Literal, RhsPos: name.Pos}
			continue
		}

		// subscript or slice access
		if next.IsSimple("[") {
			ps.NextToken() // consume '['
			if ps.consume(":") {
				var end ast.Node
				if !ps.PeekToken().IsSimple("]") {
					end = ps.parse(0, true)
				}
				if !ps.consume("]") {
					ps.expected("']'", next, "this '[' opens the slice")
				}
				left = ast.Slice{Pos: left.Where().To(ps.last.Pos), Lhs: left, End: end}
				continue
			}

			first := ps.parse(0, true)
			if ps.consume(":") {
				var end ast.Node
				if !ps.PeekToken().IsSimple("]") {
					end = ps.parse(0, true)
				}
				if !ps.consume("]") {
					ps.expected("']'", next, "this '[' opens the slice")
				}
				left = ast.Slice{Pos: left.Where().To(ps.last.Pos), Lhs: left, Start: first, End: end}
				continue
			}

			if !ps.consume("]") {
				ps.expected("']'", next, "this '[' opens the index")
			}
			left = ast.Subscript{Pos: left.Where().To(ps.last.Pos), Lhs: left, Key: first}
			continue
		}

		// existence check
		if next.IsSimple("?") {
			ps.NextToken() // consume '?'
			left = ast.Exists{Pos: left.Where().To(next.Pos), Value: left}
			continue
		}

		// instance check
		if next.IsWord("is") {
			if isPrec < precedenceLevel {
				break
			}
			ps.NextToken() // consume 'is'
			rhs := ps.parse(isPrec+1, true)
			left = ast.IsInstanceOf{Pos: left.Where().To(rhs.Where()), Lhs: left, Rhs: rhs}
			continue
		}

		// function call
		if next.IsSimple("(") {
			ps.NextToken() // consume '('
			args := ps.parseArgs(next, "the call")
			left = ast.Call{Pos: left.Where().To(ps.last.Pos), Fn: left, Args: args}
			continue
		}

		if next.Type != token.Simple {
			break
		}
		currentPrecedence, ok := precedence[next.Literal]
		// stop parsing if next token is not an infix operator or precedence is lower
		if !ok || currentPrecedence < precedenceLevel {
			break
		}

		// consume the operator
		ps.NextToken()
		// parse the right-hand side with higher precedence level
		right := ps.parse(currentPrecedence+1, true)
		left = ast.BinOp{Pos: left.Where().To(right.Where()), Lhs: left, Operator: operators[next.Literal], Rhs: right}
	}
	return left
}
