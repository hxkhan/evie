package vm

import (
	"fmt"
	"runtime/debug"
)

type Error struct {
	Message string
	Code    int
}

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

// recovered turns a panic that reached a host boundary into an exception. A
// thrown exception or compile error passes through; anything else is a bug in
// the runtime or in a native and is reported with its stack instead of taking
// the host process down.
func recovered(r any) Exception {
	switch e := r.(type) {
	case Exception:
		return e
	case *CompileError:
		return &Error{Message: e.Error()}
	}
	return &Error{Message: fmt.Sprintf("internal error: %v\n%s", r, debug.Stack())}
}

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

// CompileError represents an error that occurred during compilation or static analysis.
type CompileError struct {
	Message string
	Line    int
}

func (e CompileError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("compile error on line %d: %s", e.Line, e.Message)
	}
	return fmt.Sprintf("compile error: %s", e.Message)
}

func CompileErrorf(format string, a ...any) *CompileError {
	return &CompileError{
		Message: fmt.Sprintf(format, a...),
	}
}
