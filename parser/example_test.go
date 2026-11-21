package parser_test

import (
	"fmt"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/parser"
)

func ExampleParse() {
	node, err := parser.Parse([]byte("package main\nfn add(a, b) => a + b"))
	if err != nil {
		panic(err)
	}
	pkg := node.(ast.Package)
	fmt.Printf("package=%s decls=%d\n", pkg.Name, len(pkg.Code))
	// Output:
	// package=main decls=1
}
