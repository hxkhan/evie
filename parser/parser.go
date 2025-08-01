package parser

import (
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/lexer"
	"github.com/hxkhan/evie/token"
)

var errEOS = errors.New("EOS")

type parser struct {
	*lexer.Lexer
	last token.Token
}

var keywords = []string{"package", "null", "true", "false", "fn", "return", "go", "await", "echo", "if", "else"}

var operators = map[string]ast.Operator{
	"+": ast.AddOp, "-": ast.SubOp, "*": ast.MulOp, "/": ast.DivOp,
	"+=": ast.AddOp, "-=": ast.SubOp, "*=": ast.MulOp, "/=": ast.DivOp,
	"==": ast.EqOp, "<": ast.LtOp, ">": ast.GtOp,
}

var precedence = map[string]int{"+": 1, "-": 1, "*": 2, "/": 2, "==": 0, "<": 0, ">": 0}

func Parse(input []byte) (node ast.Node, err error) {
	ps := parser{Lexer: lexer.New(input)}
	var pack ast.Package
	var cb ast.Block

	defer func() {
		if r := recover(); r != nil {
			switch r {
			case errEOS:
				if len(cb.Code) == 0 && pack.Name != "" {
					node = pack
				} else if len(cb.Code) > 0 {
					node = cb
				} else {
					err = errors.New("ParseError: invalid input")
				}
			default:
				if rErr, ok := r.(error); ok {
					err = fmt.Errorf("ParseError: %s", rErr.Error())
				} else {
					panic(r)
				}
			}
		}
	}()

	if ps.consume("package") {
		// create package & parse imports
		pack = ps.parsePackage()
		for {
			pack.Code = append(pack.Code, ps.next())
		}
	}

	// no package, just code
	for {
		cb.Code = append(cb.Code, ps.next())
	}
}

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

func (ps *parser) parsePackage() ast.Package {
	line := ps.last.Line
	if ps.PeekToken().Type != token.Word {
		panic(fmt.Errorf("expected name after package on line %v, got '%v'", line, ps.PeekToken().Literal))
	}

	pack := ast.Package{Pos: line, Name: ps.NextToken().Literal}
	if ps.consumeName("imports") && ps.consume("(") && !ps.consume(")") {
		pack.Imports = ps.parseStringList()
	}
	return pack
}

// helper to parse comma-separated string lists
func (ps *parser) parseStringList() []string {
	var imports []string
	for {
		next := ps.NextToken()
		if next.Type != token.String {
			panic(fmt.Errorf("expected string on line %v, got '%v'", next.Line, next.Literal))
		}

		// success: add import
		imports = append(imports, next.Literal)

		if ps.consume(")") {
			break
		}
		if !ps.consume(",") {
			panic(fmt.Errorf("expected ',' or ')' on line %v, got '%v'", next.Line, ps.PeekToken().Literal))
		}
	}
	return imports
}

func (ps *parser) panic(main token.Token, expected string) {
	context := map[string]string{
		"fn": "function", "if": "if statement", "else": "else statement", ".": "operator '.'",
	}
	what := context[main.Literal]
	if what == "" {
		what = main.Literal
	}
	panic(fmt.Errorf("%v on line %v expected %v, got '%v'", what, main.Line, expected, ps.PeekToken().Literal))
}

func (ps *parser) next() ast.Node {
	main := ps.PeekToken()
	if main.IsEOS() {
		panic(errEOS)
	}

	switch {
	case main.Type == token.Word:
		return ps.handleWords(ps.NextToken())
	case main.Type == token.String:
		return ast.Input[string]{Pos: main.Line, Value: ps.NextToken().Literal}
	case main.Type == token.Number:
		return ast.Input[float64]{Pos: main.Line, Value: ps.parseFloat(ps.NextToken().Literal)}
	case main.IsSimple("-"):
		return ps.parseNegation(main)
	default:
		return ps.parseExpression(0)
	}
}

func (ps *parser) parseFloat(literal string) float64 {
	num, err := strconv.ParseFloat(literal, 64)
	if err != nil {
		panic(fmt.Errorf("error parsing number: %v", err))
	}
	return num
}

func (ps *parser) parseNegation(main token.Token) ast.Node {
	ps.NextToken()
	neg := ast.Neg{Pos: main.Line}
	if ps.PeekToken().Type == token.Number {
		neg.O = ast.Input[float64]{Pos: main.Line, Value: ps.parseFloat(ps.NextToken().Literal)}
	} else {
		neg.O = ps.parseExpression(0)
	}
	return neg
}

func (ps *parser) handleWords(main token.Token) ast.Node {
	switch main.Literal {
	case "echo":
		return ast.Echo{Pos: main.Line, Value: ps.parseExpression(0)}
	case "fn":
		return ps.parseFn(main)
	case "go":
		return ast.Go{Pos: main.Line, Fn: ps.parseExpression(0)}
	case "await":
		return ps.parseAwait(main)
	case "null":
		return ast.Input[struct{}]{Pos: main.Line}
	case "true":
		return ast.Input[bool]{Pos: main.Line, Value: true}
	case "false":
		return ast.Input[bool]{Pos: main.Line, Value: false}
	case "if":
		return ps.parseConditional(main)
	case "while":
		return ps.parseWhile(main)
	case "return":
		ret := ast.Return{Pos: main.Line}
		if !ps.PeekToken().IsSimple("}") {
			ret.Value = ps.parseExpression(0)
		} else {
			ret.Value = ast.Input[struct{}]{Pos: main.Line}
		}
		return ret
	case "var":
		name := ps.NextToken()
		if !ps.consume(":=") {
			panic(fmt.Errorf("expected ':=' after 'var %v' on line %v, got '%v' instead", name.Literal, main.Line, ps.PeekToken().Literal))
		}
		return ast.Decl{Pos: main.Line, Name: name.Literal, IsStatic: false, Value: ps.parseExpression(0)}
	case "pub":
		// skip for now
		return ps.next()

	default:
		return ps.parseIdentOrCall(main)
	}
}

func (ps *parser) parseAwait(main token.Token) ast.Node {
	// await.all(x, y, z) or await.any(x, y, z)
	if !ps.consume(".") {
		return ast.Await{Task: ps.parseExpression(0)}
	}

	if ps.consumeName("all") {
		return ast.AwaitAll{Pos: main.Line, Names: ps.parseNamesList(ps.last)}
	}
	if ps.consumeName("any") {
		return ast.AwaitAny{Pos: main.Line, Names: ps.parseNamesList(ps.last)}
	}
	return ast.Await{Task: ps.parseExpression(0)}
}

func (ps *parser) parseIdentOrCall(main token.Token) ast.Node {
	// handle const declarations explicitly
	if ps.consume(":=") {
		return ast.Decl{Pos: main.Line, Name: main.Literal, IsStatic: true, Value: ps.parseExpression(0)}
	}

	// try infix stuff
	left := ps.parseInfixExpression(ast.Ident{Pos: main.Line, Name: main.Literal}, 0)

	if ps.consume("=") {
		return ast.Assign{Pos: main.Line, Lhs: left, Value: ps.parseExpression(0)}
	}
	if ps.consume("+=") || ps.consume("-=") {
		return ast.Assign{Pos: main.Line, Lhs: left, Value: ast.BinOp{
			Pos: main.Line, Lhs: ast.Ident{Name: main.Literal},
			Operator: operators[ps.last.Literal], Rhs: ps.parseExpression(0),
		}}
	}
	if ps.consume("(") {
		return ast.Call{Pos: main.Line, Fn: left, Args: ps.parseArgsList()}
	}
	return left
}

func (ps *parser) parseConditional(main token.Token) ast.Node {
	node := ast.Conditional{Pos: main.Line}
	node.Condition = ps.parseExpression(0)
	if !ps.consume("{") {
		ps.panic(main, "'{'")
	}
	node.Action = ps.parseBlock()
	if ps.consume("else") {
		if ps.consume("if") {
			node.Otherwise = ps.parseConditional(main)
		} else {
			if !ps.consume("{") {
				ps.panic(ps.last, "'{'")
			}
			node.Otherwise = ps.parseBlock()
		}
	}
	return node
}

func (ps *parser) parseWhile(main token.Token) ast.Node {
	node := ast.While{Pos: main.Line}
	node.Condition = ps.parseExpression(0)
	if !ps.consume("{") {
		ps.panic(main, "'{'")
	}
	node.Action = ps.parseBlock()
	return node
}

// helper to parse a block or single statement
func (ps *parser) parseBlock() ast.Node {
	var block ast.Block
	for !ps.consume("}") {
		block.Code = append(block.Code, ps.next())
	}
	return block
}

// helper to parse an fn
func (ps *parser) parseFn(main token.Token) ast.Node {
	fn := ast.Fn{Pos: main.Line}
	if ps.PeekToken().Type == token.Word {
		fn.Name = ps.NextToken().Literal
	}
	fn.Args = ps.parseNamesList(main)

	if ps.consume("{") {
		fn.Action = ps.parseBlock()
	} else if ps.consume("=>") {
		// TODO: we should check so next already isn't a return
		fn.Action = ast.Return{Pos: main.Line, Value: ps.next()}
	} else {
		ps.panic(main, "'{' or '=>'")
	}
	return fn
}

// helper to parse names surrounded by parentheses
func (ps *parser) parseNamesList(main token.Token) []string {
	if !ps.consume("(") {
		ps.panic(main, "'('")
	}

	var args []string
	if ps.consume(")") {
		return args
	}

	for {
		if ps.PeekToken().Type != token.Word {
			ps.panic(main, "names in parentheses")
		}
		args = append(args, ps.NextToken().Literal)

		if ps.consume(")") {
			break
		}
		if !ps.consume(",") {
			ps.panic(main, "',' or ')'")
		}
	}
	return args
}

func (ps *parser) parseArgsList() []ast.Node {
	var args []ast.Node
	if ps.consume(")") {
		return args
	}

	for {
		args = append(args, ps.parseExpression(0))
		if ps.consume(")") {
			break
		}
		if !ps.consume(",") {
			panic(fmt.Errorf("expected ',' or ')' on line %v", ps.PeekToken().Line))
		}
	}
	return args
}

func (ps *parser) parseExpression(precedenceLevel int) ast.Node {
	// handle parentheses explicitly
	if ps.consume("(") {
		line := ps.last.Line
		// reset precedence to 0 inside parentheses
		expr := ps.parseExpression(0)
		// ensure the closing parenthesis is present
		if !ps.consume(")") {
			panic(fmt.Errorf("'(' expected ')' on line %v, got '%v'", line, ps.PeekToken().Literal))
		}
		// the sub-expression within the parentheses becomes the new left
		return ps.parseInfixExpression(expr, precedenceLevel)
	}
	// parse the left-hand side
	return ps.parseInfixExpression(ps.next(), precedenceLevel)
}

func (ps *parser) parseInfixExpression(left ast.Node, precedenceLevel int) ast.Node {
	for {
		next := ps.PeekToken()

		// field access
		if next.IsSimple(".") {
			ps.NextToken()
			if ps.PeekToken().Type != token.Word {
				panic(fmt.Errorf("dot operator expected name on line %v", next.Line))
			}
			left = ast.FieldAccess{Pos: next.Line, Lhs: left, Rhs: ps.NextToken().Literal}
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
		right := ps.parseExpression(currentPrecedence + 1)
		left = ast.BinOp{Pos: next.Line, Lhs: left, Operator: operators[next.Literal], Rhs: right}
	}
	return left
}
