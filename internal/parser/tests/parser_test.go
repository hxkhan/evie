package tests

import (
	"fmt"
	"testing"

	"github.com/hk-32/evie"
	"github.com/hk-32/evie/core"
	"github.com/hk-32/evie/internal/ast"
	"github.com/hk-32/evie/internal/parser"
)

type ReturnTestCase struct {
	input  string
	expect string
}

func TestBinaryPrecedence(t *testing.T) {
	var tests = []ReturnTestCase{
		// Basic addition and multiplication
		{"6 * 2 + 1", "13"},
		{"1 + 2 * 6", "13"},

		// Parentheses precedence
		{"(6 + 2) * 2", "16"},
		{"2 * (6 + 2)", "16"},

		// Division and addition
		{"10 / 2 + 2", "7"},
		{"2 + 3 * 4", "14"},

		// Subtraction and multiplication
		{"10 - 2 * 3", "4"},
		{"(10 - 2) * 3", "24"},

		// Division and subtraction
		{"18 / 3 - 2", "4"},
		{"(18 - 6) / 3", "4"},

		// Combined operations
		{"10 * 2 + 3 - 5 / 2", "20.5"},
		{"10 / 2 * 3 + 1", "16"},
		{"6 + 3 * 4 / 2 - 5", "7"},

		// Multiple parentheses
		{"(2 + 3) * (5 - 2)", "15"},
		{"((2 + 3) * 5) - (8 / 2)", "21"},

		// Negative numbers
		{"-5 + 3", "-2"},
		{"-3 * 4", "-12"},
		{"-(4 + 6) * 2", "-20"},
	}

	//testRTC(ReturnTestCase{"10 * 2 + 3 - 5 / 2", "20"})

	for i, test := range tests {
		err := testRTC(test)
		if err != nil {
			t.Fatalf("tests[%v] %v got %v", i, test.input, err.Error())
		}
	}
}

func testRTC(rtc ReturnTestCase) error {
	prog := fmt.Sprintf(`package main
fn main() {
	return %s
}`, rtc.input)

	nodes, err := parser.Parse([]byte(prog))
	if err != nil {
		return fmt.Errorf("failed to parse with %v", err)
	}

	program, err := ast.Compile(nodes, true, evie.DefaultExports())
	if err != nil {
		return fmt.Errorf("failed to compile with %v", err)
	}

	errInit := program.Initialize()
	if errInit != nil {
		return errInit
	}

	fn, _ := core.GetGlobal("main").AsUserFn()
	res, err := fn.Call()

	if err != nil {
		if rtc.expect == "error" {
			return nil // Expected an error, so this is considered a success
		}
		return fmt.Errorf("failed to run with %v", err)
	}

	if result := fmt.Sprint(res); result != rtc.expect {
		return fmt.Errorf("expected %v but got %v", rtc.expect, result)
	}

	return nil
}
