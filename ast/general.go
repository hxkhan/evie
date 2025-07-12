package ast

import (
	"fmt"

	"github.com/hk-32/evie/core"
)

/* IDEA: Add ensureReachabilty() to Node so this gives an error at compile time
DECL printer main(6) // inc is uninitialized when main is called
DECL inc 10


FN main(x)
    STORE inc FN()
        STORE x ADD x 1
    END

    RET FN()
        OUT x
    END
END

// Will also only be used in global scope so this shouldn't be too expensive
*/

type Node interface {
	compile(cs *Machine)
}

type Input struct {
	Value any
}

func (in Input) compile(cs *Machine) {
	switch v := in.Value.(type) {
	case nil:
		cs.emit(func(rt *core.CoRoutine) (int, error) {
			rt.Stack = append(rt.Stack, core.Value{})
			return 1, nil
		})
	case string:
		cs.emit(func(rt *core.CoRoutine) (int, error) {
			rt.Stack = append(rt.Stack, core.BoxString(v))
			return 1, nil
		})
	case int64:
		cs.emit(func(rt *core.CoRoutine) (int, error) {
			rt.Stack = append(rt.Stack, core.BoxInt64(v))
			return 1, nil
		})
	case float64:
		cs.emit(func(rt *core.CoRoutine) (int, error) {
			rt.Stack = append(rt.Stack, core.BoxFloat64(v))
			return 1, nil
		})
	case bool:
		cs.emit(func(rt *core.CoRoutine) (int, error) {
			rt.Stack = append(rt.Stack, core.BoxBool(v))
			return 1, nil
		})
	}
}

type Block []Node

func (b Block) compile(cs *Machine) {
	for _, statement := range b {
		statement.compile(cs)
	}
}

type Echo struct {
	Value Node
}

func (out Echo) compile(cs *Machine) {
	out.Value.compile(cs)

	cs.emit(func(rt *core.CoRoutine) (int, error) {
		v := rt.Stack[len(rt.Stack)-1]
		fmt.Println(v)
		return 1, nil
	})
}
