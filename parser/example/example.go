package main

import (
	"fmt"
	"os"

	"hxkhan.dev/evie/parser"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("No input file provided.")
		return
	}

	input, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}

	node, err := parser.Parse(input)
	if err != nil {
		panic(err)
	}

	fmt.Println(node)
}
