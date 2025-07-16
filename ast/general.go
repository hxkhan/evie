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
	compile(vm *Machine) core.Instruction
}

type Input struct {
	Value core.Value
}

func (in Input) compile(vm *Machine) core.Instruction {
	return func(rt *core.CoRoutine) (core.Value, error) {
		return in.Value, nil
	}
}

type Block []Node

func (b Block) compile(vm *Machine) core.Instruction {
	block := make([]core.Instruction, len(b))
	for i, statement := range b {
		block[i] = statement.compile(vm)
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

func (out Echo) compile(vm *Machine) core.Instruction {
	what := out.Value.compile(vm)

	return func(rt *core.CoRoutine) (core.Value, error) {
		v, err := what(rt)
		if err != nil {
			return v, err
		}

		fmt.Println(v)
		return core.Value{}, nil
	}
}
