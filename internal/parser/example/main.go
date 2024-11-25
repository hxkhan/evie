package main

import (
	"fmt"
	"os"

	"github.com/hk-32/evie"
	"github.com/hk-32/evie/internal/ast"
	"github.com/hk-32/evie/internal/parser"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("No input file provided.")
		return
	}

	input, err := os.ReadFile(os.Args[1]) // ./lexer/test/input.hx
	if err != nil {
		panic(err)
	}

	nodes, err := parser.Parse(input)
	if err != nil {
		fmt.Println(err)
		return
	}

	program, err := ast.Compile(nodes, true, evie.DefaultExports())
	if err != nil {
		fmt.Println(err)
		return
	}

	program.PrintCode()
	fmt.Println("------------------------------")

	res, err := program.Start()
	if err != nil {
		fmt.Println(err)
		return
	}

	if !res.IsNull() {
		fmt.Println(res)
	}
}
