package vm

import (
	"errors"
	"fmt"
	"strings"

	"github.com/hxkhan/evie/vm/fields"
)

var exception = &UserStruct{
	Name: "error",
	Fields: []fields.ID{
		fields.Get("kind"),
		fields.Get("message"),
	},
}

var signalReturn Exception = &UserStructInstance{
	InstanceOf: exception,
	Fields: map[fields.ID]Value{
		fields.Get("kind"):    BoxString("Signal"),
		fields.Get("message"): BoxString("return"),
	},
}

var signalContinue Exception = &UserStructInstance{
	InstanceOf: exception,
	Fields: map[fields.ID]Value{
		fields.Get("kind"):    BoxString("Signal"),
		fields.Get("message"): BoxString("continue"),
	},
}

var signalBreak Exception = &UserStructInstance{
	InstanceOf: exception,
	Fields: map[fields.ID]Value{
		fields.Get("kind"):    BoxString("Signal"),
		fields.Get("message"): BoxString("break"),
	},
}

type Exception = *UserStructInstance

func (e Exception) Error() string {
	if e.InstanceOf == exception {
		kind := e.Fields[fields.Get("kind")]
		message := e.Fields[fields.Get("message")]

		return fmt.Sprintf("%v: %v", kind, message)
	}
	return e.String()
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

var ErrTypes Exception = &UserStructInstance{
	InstanceOf: exception,
	Fields: map[fields.ID]Value{
		fields.Get("kind"):    BoxString("TypeError"),
		fields.Get("message"): BoxString("wrong type of arguments given to function"),
	},
}

func CustomError(msg string, a ...any) Exception {
	return &UserStructInstance{
		InstanceOf: exception,
		Fields: map[fields.ID]Value{
			fields.Get("kind"):    BoxString("TypeError"),
			fields.Get("message"): BoxString(fmt.Sprintf(msg, a...)),
		},
	}
}

func operatorError(op string, a Value, b Value) Exception {
	return &UserStructInstance{
		InstanceOf: exception,
		Fields: map[fields.ID]Value{
			fields.Get("kind"):    BoxString("TypeError"),
			fields.Get("message"): BoxString(fmt.Sprintf("cannot apply '%v' operator on types '%v' and '%v'", op, a.TypeOf(), b.TypeOf())),
		},
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

	return &UserStructInstance{
		InstanceOf: exception,
		Fields: map[fields.ID]Value{
			fields.Get("kind"):    BoxString("TypeError"),
			fields.Get("message"): BoxString("TypeError: " + msg + ")"),
		},
	}
}

func RuntimeExceptionF(format string, a ...any) Exception {
	return &UserStructInstance{
		InstanceOf: exception,
		Fields: map[fields.ID]Value{
			fields.Get("kind"):    BoxString("RuntimeException"),
			fields.Get("message"): BoxString(fmt.Sprintf(format, a...)),
		},
	}
}

func TypeErrorF(format string, a ...any) Exception {
	return &UserStructInstance{
		InstanceOf: exception,
		Fields: map[fields.ID]Value{
			fields.Get("kind"):    BoxString("TypeError"),
			fields.Get("message"): BoxString(fmt.Sprintf(format, a...)),
		},
	}
}
