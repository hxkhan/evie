package vm

import (
	"errors"
	"fmt"
	"strings"
)

type Exception struct {
	name    string
	message string
}

func (e Exception) Error() string {
	return e.name + ": " + e.message
}

var returnSignal = &Exception{name: "signal", message: "return"}
var notFunction = &Exception{name: "signal", message: "not a function"}

type trace struct {
	err   error
	stack []string
}

func (t trace) Error() string {
	var bytes strings.Builder

	bytes.WriteString(t.err.Error())
	bytes.WriteByte('\n')
	for _, name := range t.stack {
		bytes.WriteString(fmt.Sprintf("\tin '%v'\n", name))
	}

	return bytes.String()
}

var ErrNotCallable error = errors.New("not a callable")

var ErrTypes = &Exception{name: "TypeError", message: "wrong type of arguments given to function"}

func CustomError(msg string, a ...any) *Exception {
	return &Exception{"RuntimeError", fmt.Sprintf(msg, a...)}
}

func operatorError(op string, a any, b any) *Exception {
	return &Exception{"RuntimeError", fmt.Sprintf("cannot apply '%v' operator on '%v' and '%v'", op, a, b)}
}

func TypeError(args []Value, expected ...string) *Exception {
	msg := "expected types ("
	for i, ex := range expected {
		msg += fmt.Sprintf("'%s'", ex)
		if i != len(expected)-1 {
			msg += ", "
		}
	}

	msg += "), got ("
	for i, arg := range args {
		msg += fmt.Sprintf("'%s'", arg.TypeOf())
		if i != len(args)-1 {
			msg += ", "
		}
	}

	return &Exception{"TypeError", msg + ")"}
}

func RuntimeExceptionF(format string, a ...any) *Exception {
	return &Exception{name: "RuntimeException", message: fmt.Sprintf(format, a...)}
}

func TypeErrorF(format string, a ...any) *Exception {
	return &Exception{name: "TypeError", message: fmt.Sprintf(format, a...)}
}
