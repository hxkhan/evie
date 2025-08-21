package main

import (
	"fmt"
	"os"

	"hxkhan.dev/evie/lexer"
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

	width := digits(countLines(input))

	lex := lexer.New(input)

	for {
		v := lex.NextToken()
		if v.IsEOS() {
			break
		}

		fmt.Printf("%v : %-8v -> %-20v\n", padding_left(int(v.Line), width), v.Type, v.Literal)
	}
}

func duplicate(x byte, num int) string {
	container := make([]byte, num)
	for n := range num {
		container[n] = x
	}
	return string(container)
}

func padding_left(x int, space int) string {
	op := fmt.Sprint(x)

	if len(op) < space {
		amount := space - len(op)
		return duplicate('0', amount) + op
	}
	return op
}

func countLines(input []byte) int {
	n := 1
	for _, v := range input {
		if v == '\n' {
			n++
		}
	}
	return n
}

func digits(x int) int {
	if x == 0 {
		return 1
	}
	count := 0
	for x > 0 || x < 0 {
		x = x / 10
		count++
	}
	return count
}
