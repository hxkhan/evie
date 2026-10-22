package parser

import (
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/lexer"
	"github.com/hxkhan/evie/token"
)

var errEOS = errors.New("EOS")

// Error is a syntax error: a report with the offending span and, where it
// helps, the construct that was left open
type Error struct {
	diag.Report
}

func (e *Error) Error() string {
	return "syntax error: " + e.Report.Error()
}

type parser struct {
	*lexer.Lexer
	src  *diag.Source
	last token.Token
}

var keywords = []string{
	"package", "imports",
	"nil", "true", "false",
	"var", "const", "fn", "struct",
	"echo",
	"return",
	"if", "else",
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
	"<":  2, ">": 2, "==": 2, "!=": 2, "<=": 2, ">=": 2, "is": 2,
	"+": 3, "-": 3,
	"*": 4, "/": 4, "%": 4,
	".": 5,
	"(": 6,
}

// Parse parses a script whose source has no name
func Parse(input []byte) (ast.Node, error) {
	return ParseSource(diag.NewSource("", input))
}

// ParseSource parses a script; errors name the source and show its text
func ParseSource(src *diag.Source) (node ast.Node, err error) {
	ps := parser{Lexer: lexer.New(src.Text), src: src}
	var pack ast.Package
	var cb ast.Block

	defer func() {
		if r := recover(); r != nil {
			switch e := r.(type) {
			case *Error:
				err = e
			case runtime.Error:
				// a bug in the parser or lexer; report it instead of taking the host down
				err = fmt.Errorf("internal parser error: %v", e)
			case error:
				if e == errEOS {
					if len(cb.Code) == 0 && pack.Name != "" {
						node = pack
					} else if len(cb.Code) > 0 {
						node = cb
					} else {
						err = ps.report(ps.PeekToken().Pos, "the source is empty")
					}
					return
				}
				err = fmt.Errorf("ParseError: %s", e.Error())
			default:
				err = ps.report(ps.last.Pos, fmt.Sprint(r))
			}
		}
	}()

	if ps.consume("package") {
		// create package & parse imports
		pack = ps.parsePackage()
		for {
			pack.Code = append(pack.Code, ps.parse(0, false))
		}
	}

	// no package, just code
	for {
		cb.Code = append(cb.Code, ps.parse(0, false))
	}
}

// report builds a syntax error at a position
func (ps *parser) report(pos token.Pos, msg string, notes ...diag.Note) *Error {
	return &Error{Report: diag.Report{Message: msg, Source: ps.src, Pos: pos, Notes: notes}}
}

// fail stops parsing with an error at a position
func (ps *parser) fail(pos token.Pos, msg string, notes ...diag.Note) {
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
	pack := ast.Package{Pos: keyword.Pos.To(name.Pos), Name: name.Literal}
	if ps.consume("imports") {
		imports := ps.last
		if !ps.consume("(") {
			ps.expected("'(' after imports", imports, "")
		}
		open := ps.last
		if !ps.consume(")") {
			pack.Imports = ps.parseStringList(open)
		}
	}
	return pack
}

// helper to parse comma-separated string lists
func (ps *parser) parseStringList(open token.Token) []string {
	var imports []string
	for {
		next := ps.PeekToken()
		if next.Type != token.String {
			ps.expected("a package name in quotes", open, "in this import list")
		}
		ps.NextToken()
		imports = append(imports, next.Literal)

		if ps.consume(")") {
			break
		}
		if !ps.consume(",") {
			ps.expected("',' or ')'", open, "this '(' opens the import list")
		}
	}
	return imports
}

func (ps *parser) parseFloat(tok token.Token) float64 {
	num, err := strconv.ParseFloat(tok.Literal, 64)
	if err != nil {
		ps.fail(tok.Pos, fmt.Sprintf("'%s' is not a valid number", tok.Literal))
	}
	return num
}

func (ps *parser) handleWords(main token.Token, asExpr bool, precedenceLevel int) ast.Node {
	switch main.Literal {
	case "echo":
		return ast.Echo{Pos: main.Pos, Value: ps.parse(0, true)}
	case "fn":
		return ps.parseFn(main, asExpr)
	case "struct":
		return ps.parseStruct(main)
	case "go":
		return ast.Go{Pos: main.Pos, Fn: ps.parse(0, true)}
	case "await":
		return ps.parseAwait(main)
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
		if !ps.PeekToken().IsSimple("}") {
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
		if ps.PeekToken().Line == main.Line && !ps.PeekToken().IsSimple("}") {
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
	format := strings.Builder{}
	st.Pos = main.Pos

	for {
		if ps.consume("`") {
			break
		} else if ps.consume("{") {
			lb := ps.last
			st.Args = append(st.Args, ps.parse(0, true))
			if !ps.consume("}") {
				ps.expected("'}'", lb, "this '{' opens an interpolation")
			}
			format.WriteString("%s")
		} else if ps.PeekToken().Type == token.String {
			part := ps.NextToken()
			text := ps.unescape(part.Literal, part.Pos, "`{}")
			format.WriteString(strings.ReplaceAll(text, "%", "%%"))
		} else {
			ps.expected("the closing '`'", main, "this template string is unterminated")
		}
	}

	st.Pos = main.Pos.To(ps.last.Pos)
	st.Format = format.String()
	return st
}

func (ps *parser) parseAwait(main token.Token) ast.Node {
	// await.all(x, y, z), await.any(x, y, z), or await.race(x, y, z)
	if !ps.consume(".") {
		task := ps.parse(0, true)
		return ast.Await{Pos: main.Pos.To(task.Where()), Task: task}
	}

	if ps.consumeName("all") {
		return ast.AwaitAll{Pos: main.Pos, Tasks: ps.parseArgsList()}
	}
	if ps.consumeName("any") {
		return ast.AwaitAny{Pos: main.Pos, Tasks: ps.parseArgsList()}
	}
	if ps.consumeName("race") {
		return ast.AwaitRace{Pos: main.Pos, Tasks: ps.parseArgsList()}
	}
	ps.expected("'all', 'any' or 'race' after 'await.'", main, "")
	return nil
}

func (ps *parser) parseIdent(main token.Token, precedenceLevel int) ast.Node {
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
			node.Otherwise = ps.parseConditional(ps.last)
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
	if ps.consume(",") {
		indexVar = first.Literal
		itemVar = ps.name("the item variable after ','").Literal
	} else {
		itemVar = first.Literal
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
		ItemVar:    itemVar,
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
			block.Code = append(block.Code, ps.parseNode(0, true))
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

	return sw
}

// parseBlock parses statements up to the '}' that closes open
func (ps *parser) parseBlock(open token.Token) ast.Node {
	block := ast.Block{Pos: open.Pos}
	for !ps.consume("}") {
		if ps.PeekToken().IsEOS() {
			ps.expected("'}'", open, "this '{' is never closed")
		}
		block.Code = append(block.Code, ps.parseNode(0, true))
	}
	return block
}

// helper to parse an fn
func (ps *parser) parseFn(main token.Token, asExpr bool) ast.Node {
	fn := ast.Fn{Pos: main.Pos, UsedAsExpr: asExpr}
	if ps.PeekToken().Type == token.Word {
		name := ps.NextToken()
		fn.Name = name.Literal
		fn.Pos = main.Pos.To(name.Pos)
	}
	fn.Args, fn.IsVariadic = ps.parseNamesList(main, "the parameter list")

	if ps.consume("{") {
		fn.Action = ps.parseBlock(ps.last)
	} else if ps.consume("=>") {
		// an arrow body may also be an assignment, as in `fn reset() => this.n = 0`
		body := ps.parseNode(0, true)
		ps.mustBeValue(body, true)
		fn.Action = ast.Return{Pos: ps.last.Pos, Value: body}
	} else {
		ps.expected("'{' or '=>' after the parameters", main, "in this function")
	}
	return fn
}

// helper to parse a struct
func (ps *parser) parseStruct(main token.Token) ast.Node {
	str := ast.StructDefinition{Pos: main.Pos}
	if ps.PeekToken().Type == token.Word {
		name := ps.NextToken()
		str.Name = name.Literal
		str.Pos = main.Pos.To(name.Pos)
	}
	var isVariadic bool
	str.Args, isVariadic = ps.parseNamesList(main, "the field list")
	if isVariadic {
		ps.fail(ps.last.Pos, "struct fields cannot be variadic")
	}

	if ps.consume(":") {
		str.Parent = ps.name("the parent struct after ':'").Literal
	}

	if !ps.consume("{") {
		ps.expected("'{' for the struct body", main, "in this struct")
	}
	str.Action = ps.parseBlock(ps.last)

	return str
}

// helper to parse names surrounded by parentheses
func (ps *parser) parseNamesList(main token.Token, what string) ([]string, bool) {
	if !ps.consume("(") {
		ps.expected("'(' to open "+what, main, "")
	}
	open := ps.last

	var args []string
	if ps.consume(")") {
		return args, false
	}

	isVariadic := false
	for {
		if ps.consume("...") {
			isVariadic = true
			args = append(args, ps.name("a parameter name after '...'").Literal)
			if !ps.consume(")") {
				ps.expected("')' after the variadic parameter", open, "this '(' opens "+what)
			}
			break
		}

		args = append(args, ps.name("a name").Literal)

		if ps.consume(")") {
			break
		}
		if !ps.consume(",") {
			ps.expected("',' or ')'", open, "this '(' opens "+what)
		}
	}
	return args, isVariadic
}

func (ps *parser) parseArgsList() []ast.Node {
	if !ps.consume("(") {
		ps.expected("'('", ps.last, "")
	}
	open := ps.last

	var args []ast.Node
	if !ps.PeekToken().IsSimple(")") {
		for {
			args = append(args, ps.parse(0, true))
			if !ps.consume(",") {
				break
			}
		}
	}

	if !ps.consume(")") {
		ps.expected("',' or ')'", open, "this '(' opens the argument list")
	}

	return args
}

func (ps *parser) parseObject(main token.Token) ast.Node {
	obj := ast.Object{Pos: main.Pos}

	// empty object {}
	if ps.consume("}") {
		obj.Pos = main.Pos.To(ps.last.Pos)
		return obj
	}

	for {
		key := ps.name("a field name")
		if !ps.consume(":") {
			ps.expected(fmt.Sprintf("':' after the field name '%s'", key.Literal), main, "in this object")
		}

		value := ps.parse(0, true)
		obj.Fields = append(obj.Fields, ast.ObjectField{Key: key.Literal, Value: value})

		if ps.consume("}") {
			break
		}
		if !ps.consume(",") {
			ps.expected("',' or '}'", main, "this '{' opens the object")
		}
	}

	obj.Pos = main.Pos.To(ps.last.Pos)
	return obj
}

func (ps *parser) parseArrayLiteral(main token.Token) ast.Node {
	arr := ast.ArrayLiteral{Pos: main.Pos}

	// empty array []
	if ps.consume("]") {
		arr.Pos = main.Pos.To(ps.last.Pos)
		return arr
	}

	for {
		arr.Elements = append(arr.Elements, ps.parse(0, true))

		if ps.consume("]") {
			break
		}
		if !ps.consume(",") {
			ps.expected("',' or ']'", main, "this '[' opens the array")
		}
		if ps.consume("]") {
			break
		}
	}

	arr.Pos = main.Pos.To(ps.last.Pos)
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
	case ast.While, ast.For:
		what = "a loop"
	case ast.StructDefinition:
		what = "a struct definition"
	default:
		return
	}
	err := ps.report(node.Where(), "expected a value, found "+what)
	err.Label = "this has no value"
	err.Hint = hint
	panic(err)
}

func (ps *parser) parseNode(precedenceLevel int, asExpr bool) (node ast.Node) {
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
		// the end of the source only ends the program; inside any construct it is an error
		if asExpr {
			ps.fail(main.Pos, "unexpected end of source")
		}
		panic(errEOS)
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
			node = ps.parse(4, true)
		}

	case main.IsSimple("-"):
		ps.NextToken()
		if ps.PeekToken().Type == token.Number {
			num := ps.NextToken()
			node = ast.Input[float64]{Pos: main.Pos.To(num.Pos), Value: -ps.parseFloat(num)}
		} else {
			operand := ps.parse(4, true)
			node = ast.Neg{Pos: main.Pos.To(operand.Where()), Value: operand}
		}

	case main.IsSimple("!"):
		ps.NextToken()
		operand := ps.parse(4, true)
		node = ast.Not{Pos: main.Pos.To(operand.Where()), Value: operand}

	case main.IsSimple("`"):
		node = ps.parseStringTemplate(ps.NextToken())
	case main.Type == token.Invalid:
		if strings.HasPrefix(main.Literal, "\"") || strings.HasPrefix(main.Literal, "`") {
			ps.fail(main.Pos, "this string is never closed")
		}
		ps.fail(main.Pos, fmt.Sprintf("invalid token '%s'", main.Literal))
	default:
		if main.IsSimple(")") || main.IsSimple("]") || main.IsSimple("}") {
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

		// field access; a member may be named like a keyword, as in task.await()
		if next.IsSimple(".") {
			ps.NextToken() // consume '.'
			if ps.PeekToken().Type != token.Word {
				ps.expected("a member name after '.'", token.Token{}, "")
			}
			name := ps.NextToken()
			left = ast.FieldAccess{Pos: left.Where().To(name.Pos), Lhs: left, Rhs: name.Literal}
			continue
		}

		// subscript access
		if next.IsSimple("[") {
			ps.NextToken() // consume '['
			key := ps.parse(0, true)
			if !ps.consume("]") {
				ps.expected("']'", next, "this '[' opens the index")
			}
			left = ast.Subscript{Pos: left.Where().To(ps.last.Pos), Lhs: left, Key: key}
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
			currentPrec := precedence["is"]
			if currentPrec < precedenceLevel {
				break
			}
			ps.NextToken() // consume 'is'
			rhs := ps.parse(currentPrec+1, true)
			left = ast.IsInstanceOf{Pos: left.Where().To(rhs.Where()), Lhs: left, Rhs: rhs}
			continue
		}

		// function call
		if next.IsSimple("(") {
			ps.NextToken() // consume '('

			var args []ast.Node
			// parse arguments if any
			if !ps.PeekToken().IsSimple(")") {
				for {
					args = append(args, ps.parse(0, true))
					if !ps.consume(",") {
						break
					}
				}
			}

			if !ps.consume(")") {
				ps.expected("',' or ')'", next, "this '(' opens the call")
			}
			left = ast.Call{Pos: left.Where().To(ps.last.Pos), Fn: left, Args: args}
			continue
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
