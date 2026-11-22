package lexer_test

import (
	"fmt"

	"github.com/hxkhan/evie/internal/lexer"
)

func ExampleNew() {
	lex := lexer.New([]byte("x := 1 + 2"))
	for {
		tok := lex.NextToken()
		if tok.IsEOS() {
			break
		}
		fmt.Printf("%s(%s)\n", tok.Type, tok.Literal)
	}
	// Output:
	// word(x)
	// simple(:=)
	// number(1)
	// simple(+)
	// number(2)
}
