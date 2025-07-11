package core

import (
	"fmt"
)

func padding_left(x int, space int) string {
	ast := fmt.Sprint(x)

	if len(ast) < space {
		amount := space - len(ast)
		return duplicate('0', amount) + ast
	}
	return ast
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

func duplicate(x byte, num int) string {
	container := make([]byte, num)
	for n := 0; n < num; n++ {
		container[n] = x
	}
	return string(container)
}
