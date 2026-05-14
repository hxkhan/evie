package vm

import (
	"errors"
	"fmt"
	"strings"
)

type Error struct {
	Message string
	Code    int
}

/* var exception = &UserStruct{
	Name: "error",
	Fields: []fields.ID{
		fields.Get("kind"),
		fields.Get("message"),
	},
} */

var signalReturn = &Error{
	Message: "return",
	Code:    0,
}

var signalContinue = &Error{
	Message: "continue",
	Code:    1,
}

var signalBreak = &Error{
	Message: "break",
	Code:    2,
}

type Exception = *Error

func (e Error) Error() string {
	return e.Message
}

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

var ErrTypes Exception = &Error{
	Message: "TypeError: wrong type of arguments given to function",
}

func CustomError(msg string, a ...any) Exception {
	return &Error{
		Message: fmt.Sprintf(msg, a...),
	}
}

func operatorError(op string, a Value, b Value) Exception {
	return &Error{
		Message: fmt.Sprintf("cannot apply '%v' operator on types '%v' and '%v'", op, a.TypeOf(), b.TypeOf()),
	}
}

func TypeError(args []Value, expected ...string) Exception {
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

	return &Error{
		Message: msg,
	}
}

func RuntimeExceptionF(format string, a ...any) Exception {
	return &Error{
		Message: fmt.Sprintf(format, a...),
	}
}

func TypeErrorF(format string, a ...any) Exception {
	return &Error{
		Message: fmt.Sprintf(format, a...),
	}
}
