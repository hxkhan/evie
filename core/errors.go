package core

import (
	"errors"
	"fmt"
	"strings"
)

type errWithTrace struct {
	err   error
	trace []string
}

func (t errWithTrace) Error() string {
	var bytes strings.Builder

	bytes.WriteString(t.err.Error())
	bytes.WriteByte('\n')
	for _, name := range t.trace {
		bytes.WriteString(fmt.Sprintf("\tin '%v'\n", name))
	}

	return bytes.String()
}

var ErrReturnSignal error = errors.New("<return signal>")
var errNotFunction error = errors.New("not a native function")
var ErrNotCallable error = errors.New("not a callable")

var ErrTypes error = errors.New("wrong type of arguments given to function")

// runtime errors
type coreError struct {
	name    string
	message string
}

func (e coreError) Error() string {
	return e.name + ": " + e.message
}

func CustomError(msg string, a ...interface{}) error {
	return coreError{"RuntimeError", fmt.Sprintf(msg, a...)}
}

func OperatorTypesError(op string, a any, b any) error {
	return coreError{"RuntimeError", fmt.Sprintf("cannot apply '%v' operator on '%v' and '%v'", op, a, b)}
}

func TypeError(args []Value, expected ...string) error {
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

	return coreError{"TypeError", msg + ")"}
}
