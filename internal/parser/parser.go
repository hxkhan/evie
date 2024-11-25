package parser

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/hk-32/evie/internal/ast"
	"github.com/hk-32/evie/internal/lexer"
	"github.com/hk-32/evie/internal/op"
	"github.com/hk-32/evie/internal/token"
)

var errEOS = errors.New("EOS")

type parser struct {
	*lexer.Lexer
	lastConsumed token.Token
}

func Parse(input []byte) (node ast.Node, err error) {
	ps := parser{Lexer: lexer.New(input)}
	var pp ast.Package
	var cb ast.Block

	defer func() {
		if r := recover(); r != nil {
			if r == errEOS {
				if cb == nil {
					if pp.Name != "" {
						node = pp
					} else {
						err = errors.New("ParseError: invalid input")
					}
				} else {
					node = cb
				}
			} else if rErr, isError := r.(error); isError {
				err = fmt.Errorf("ParseError: %s", rErr.Error())
			} else {
				panic(r)
			}
		}
	}()

	if ps.consumeKeyword("package") {
		if ps.PeekToken().Type != token.Name {
			return nil, fmt.Errorf("expected a name after package on line %v, got '%v'", ps.lastConsumed.Line, ps.lastConsumed.Literal)
		}

		pp = ast.Package{Name: ps.NextToken().Literal}
		for {
			pp.Code = append(pp.Code, ps.next())
		}
	}

	// no package, just code
	for {
		cb = append(cb, ps.next())
	}
}

func (ps *parser) consumeSimple(lit string, skipNewLines bool) bool {
AGAIN:
	if ps.PeekToken().IsSimple(lit) {
		ps.lastConsumed = ps.NextToken()
		return true
	} else if skipNewLines && ps.PeekToken().IsNewLine() {
		ps.NextToken()
		goto AGAIN
	}
	return false
}

func (ps *parser) consumeKeyword(lit string) bool {
	if ps.PeekToken().IsKeyword(lit) {
		ps.lastConsumed = ps.NextToken()
		return true
	}
	return false
}

func (ps *parser) consumeName(lit string) bool {
	if ps.PeekToken().IsName(lit) {
		ps.lastConsumed = ps.NextToken()
		return true
	}
	return false
}

func (ps *parser) unexpectedPeek(main token.Token, expected string) error {
	what := ""
	switch main.Literal {
	case "fn":
		what = "function"
	case "if":
		what = "if statement"
	case "else":
		what = "else statement"
	case ".":
		what = "operator '.'"
	default:
		what = main.Literal
	}

	got := ps.PeekToken().Literal
	if got == "\n" {
		got = "\\n"
	}

	return fmt.Errorf("%v on line %v expected %v, got '%v'", what, main.Line, expected, got)
}

func (ps *parser) next() ast.Node {
	main := ps.PeekToken()

	for main.IsNewLine() {
		ps.NextToken()
		main = ps.PeekToken()
	}

	if main.IsEOS() {
		panic(errEOS)
	}

	if main.Type == token.Keyword {
		return ps.handleKeywords(ps.NextToken())
	} else if main.Type == token.Name {
		return ps.handleNames(ps.NextToken())
	} else if main.Type == token.String {
		return ast.Input{Value: ps.NextToken().Literal}
	} else if main.Type == token.Number {
		return ast.Input{Value: parseNumber(ps.NextToken().Literal)}
	} else if main.IsSimple("-") {
		ps.NextToken()

		if ps.PeekToken().Type == token.Number {
			return ast.Neg{O: ast.Input{Value: parseNumber(ps.NextToken().Literal)}}
		}

		return ast.Neg{O: ps.parseExpression(0)}
	}

	return ps.parseExpression(0)
}

func parseNumber(literal string) any {
	if strings.Contains(literal, ".") {
		num, err := strconv.ParseFloat(literal, 64)
		if err != nil {
			panic(fmt.Errorf("error when parsing number, got %v", err))
		}

		return num
	} else {
		num, err := strconv.ParseInt(literal, 10, 64)
		if err != nil {
			panic(fmt.Errorf("error when parsing number, got %v", err))
		}
		return int64(num)
	}
}

func (ps *parser) handleKeywords(main token.Token) ast.Node {
	switch main.Literal {
	case "echo":
		return ast.Echo{Value: ps.parseExpression(0)}

	case "return":
		if ps.PeekToken().IsNewLine() {
			ps.NextToken()
			return ast.Return{Value: ast.Input{Value: nil}}
		}
		return ast.Return{Value: ps.parseExpression(0)}

	case "fn":
		return ps.parseFn(main)

	case "go":
		return ast.Go{Routine: ps.parseExpression(0)}

	case "await":
		// await.all(x, y, z) or await.any(x, y, z)
		if ps.consumeSimple(".", false) {
			switch {
			case ps.consumeName("all"):
				return ast.AwaitAll{Names: ps.parseNamesList(ps.lastConsumed)}

			case ps.consumeName("any"):
				return ast.AwaitAny{Names: ps.parseNamesList(ps.lastConsumed)}
			}
		}
		return ast.Await{What: ps.parseExpression(0)}

	case "null":
		return ast.Input{Value: nil}

	case "true":
		return ast.Input{Value: true}

	case "false":
		return ast.Input{Value: false}

	case "if":
		if !ps.consumeSimple("(", false) {
			panic(ps.unexpectedPeek(main, "'('"))
		}

		condition := ps.parseExpression(0)

		if !ps.consumeSimple(")", false) {
			panic(ps.unexpectedPeek(main, ")"))
		}

		// parse the main part
		action := ps.parseBlockOrStatement()

		// parse the else part
		var otherwise ast.Node
		if ps.consumeKeyword("else") {
			otherwise = ps.parseBlockOrStatement()
		}

		return ast.Conditional{Condition: condition, Action: action, Otherwise: otherwise}
	}

	panic("unimplemented keyword?")
}

// assignments, calls, expressions etc.
func (ps *parser) handleNames(main token.Token) ast.Node {
	switch {
	case ps.consumeSimple(":=", false):
		return ast.IdentDec{Name: main.Literal, Value: ps.parseExpression(0)}

	case ps.consumeSimple("=", false):
		return ast.IdentSet{Name: main.Literal, Value: ps.parseExpression(0)}

	case ps.consumeSimple("+=", false) || ps.consumeSimple("-=", false):
		return ast.ApplyBinOp{
			OP: maps[ps.lastConsumed.Literal],
			A:  ast.IdentGet{Name: main.Literal},
			B:  ps.parseExpression(0),
		}

	case ps.consumeSimple("(", false):
		var args []ast.Node
		if !ps.consumeSimple(")", false) {
			for {
				args = append(args, ps.parseExpression(0))
				if ps.consumeSimple(")", false) {
					break
				} else if ps.consumeSimple(",", false) {
					continue
				}
				panic(fmt.Errorf("function call on line %v expected a ',' or ')', got '%v'", main.Line, ps.PeekToken().Literal))
			}
		}

		return ast.Call{Fn: ast.IdentGet{Name: main.Literal}, Args: args}
	}

	return ps.parseInfixExpression(ast.IdentGet{Name: main.Literal}, 0)
}

// helper to parse a block or single statement
func (ps *parser) parseBlockOrStatement() ast.Node {
	if ps.consumeSimple("{", true) {
		var block ast.Block

		for !ps.consumeSimple("}", true) {
			block = append(block, ps.next())
		}

		// some optimizations in the ast depend on this extraction
		if len(block) == 1 {
			return block[0]
		}
		return block
	}

	return ps.next()
}

// helper to parse an fn
func (ps *parser) parseFn(main token.Token) ast.Node {
	name := ""

	if ps.PeekToken().Type == token.Name {
		name = ps.NextToken().Literal
	}

	args := ps.parseNamesList(main)

	if ps.consumeSimple("{", false) {
		var block ast.Block

		for !ps.consumeSimple("}", true) {
			block = append(block, ps.next())
		}

		return ast.Fn{Name: name, Args: args, Action: block}
	} else if ps.consumeSimple("=>", false) {
		// maybe in the future we should check so next already isn't a return
		return ast.Fn{Name: name, Args: args, Action: ast.Return{Value: ps.next()}}
	}
	panic(ps.unexpectedPeek(main, "a '{' or '=>'"))
}

// helper to parse names surrounded by parentheses
func (ps *parser) parseNamesList(main token.Token) []string {
	if !ps.consumeSimple("(", false) {
		panic(ps.unexpectedPeek(main, "a '('"))
	}

	var args []string
	if !ps.consumeSimple(")", false) {
		for {
			if ps.PeekToken().Type != token.Name {
				panic(ps.unexpectedPeek(main, "names in parentheses"))
			}

			args = append(args, ps.NextToken().Literal)

			if ps.consumeSimple(")", false) {
				break
			} else if ps.consumeSimple(",", false) {
				continue
			}
			panic(ps.unexpectedPeek(main, "a ',' or ')'"))
		}
	}
	return args
}

// helper to parse a dot operator
func (ps *parser) parseDotOperator(left ast.Node) ast.Node {
	main := ps.NextToken() // consume the dot

	if ps.PeekToken().Type != token.Name {
		panic(ps.unexpectedPeek(main, "a name"))
	}

	name := ps.NextToken().Literal

	switch {
	case ps.consumeSimple("(", false):
		var args []ast.Node
		for !ps.PeekToken().IsSimple(")") {
			args = append(args, ps.parseExpression(0))

			if ps.consumeSimple(")", false) {
				break
			} else if ps.consumeSimple(",", false) {
				continue
			}
			panic(fmt.Errorf("method call on line %v expected a ',' or ')', got '%v'", main.Line, ps.PeekToken().Literal))
		}

		// wrap the left-hand side as the object part of the method call
		return ast.DotCall{
			Left:  left,
			Right: ast.IdentGet{Name: name},
			Args:  args,
		}
	}

	panic(ps.unexpectedPeek(main, "a '('"))
}

func (ps *parser) parseExpression(precedenceLevel int) ast.Node {
	// handle parentheses explicitly
	if ps.consumeSimple("(", false) {
		line := ps.lastConsumed.Line

		// reset precedence to 0 inside parentheses
		expr := ps.parseExpression(0)

		// ensure the closing parenthesis is present
		if !ps.consumeSimple(")", false) {
			panic(fmt.Errorf("'(' expected a ')' on line %v, got '%v'", line, ps.PeekToken().Literal))
		}

		// the sub-expression within the parentheses becomes the new left
		return ps.parseInfixExpression(expr, precedenceLevel)
	}

	// parse the left-hand side
	return ps.parseInfixExpression(ps.next(), precedenceLevel)
}

func (ps *parser) parseInfixExpression(left ast.Node, precedenceLevel int) (node ast.Node) {
	for {
		next := ps.PeekToken()

		// syntactic sugar of method calls
		if next.IsSimple(".") {
			left = ps.parseDotOperator(left)
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

		left = ast.BinOp{
			OP: maps[next.Literal],
			A:  left,
			B:  right,
		}
	}

	return left
}

var maps = map[string]byte{
	"+": op.ADD,
	"-": op.SUB,
	"*": op.MUL,
	"/": op.DIV,

	"+=": op.ADD,
	"-=": op.SUB,
	"*=": op.MUL,
	"/=": op.DIV,

	"==": op.EQ,
	"<":  op.LS,
	">":  op.MR,
}

var precedence = map[string]int{
	"+":  1,
	"-":  1,
	"*":  2,
	"/":  2,
	"==": 0,
	"<":  0,
	">":  0,
}
