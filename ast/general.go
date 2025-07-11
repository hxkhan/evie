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
	compile(cs *Machine) core.Instruction
}

type Input struct {
	Value any
}

func (in Input) compile(cs *Machine) core.Instruction {
	switch v := in.Value.(type) {
	case nil:
		return func(rt *core.CoRoutine) (core.Value, error) {
			return core.Value{}, nil
		}
	case string:
		return func(rt *core.CoRoutine) (core.Value, error) {
			return core.BoxString(v), nil
		}
	case int64:
		return func(rt *core.CoRoutine) (core.Value, error) {
			return core.BoxInt64(v), nil
		}
	case float64:
		return func(rt *core.CoRoutine) (core.Value, error) {
			return core.BoxFloat64(v), nil
		}
	case bool:
		return func(rt *core.CoRoutine) (core.Value, error) {
			return core.BoxBool(v), nil
		}
	}

	panic("Input.compile -> unimplemented type")
}

type Block []Node

func (b Block) compile(cs *Machine) core.Instruction {
	block := make([]core.Instruction, len(b))
	for i, statement := range b {
		block[i] = statement.compile(cs)
	}

	return func(rt *core.CoRoutine) (core.Value, error) {
		for _, statement := range block {
			if v, err := statement(rt); err != nil {
				return v, err
			}
		}
		return core.Value{}, nil
	}
}

type Echo struct {
	Value Node
}

func (out Echo) compile(cs *Machine) core.Instruction {
	what := out.Value.compile(cs)

	return func(rt *core.CoRoutine) (core.Value, error) {
		v, err := what(rt)
		if err != nil {
			return v, err
		}

		fmt.Println(v)
		return core.Value{}, nil
	}
}
