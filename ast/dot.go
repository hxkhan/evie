package ast

import (
	"github.com/hk-32/evie/core"
)

type DotCall struct {
	Left  Node
	Right Node
	Args  []Node
}

func (dot DotCall) compile(vm *Machine) core.Instruction {
	// namespaces e.g. json.decode(...)
	/* iGetLeft, isLeftIdentGet := dot.Left.(IdentGet)
	iGetRight, isRightIdentGet := dot.Right.(IdentGet)
	if isLeftIdentGet && isRightIdentGet {
		name := iGetLeft.Name + "." + iGetRight.Name

		if cs.isInBuiltIn(name) {
			pos := cs.emit(op.CALL, byte(len(dot.Args)))
			IdentGet{name}.compile(vm)

			for _, arg := range dot.Args {
				arg.compile(vm)
			}
			return pos
		}
	}

	// method calls
	pos := cs.emit(op.CALL, byte(len(dot.Args)+1))
	dot.Right.compile(vm) // iGet function

	dot.Left.compile(vm) // iGet
	for _, arg := range dot.Args {
		arg.compile(vm)
	}

	return pos */

	panic("implement")
}

func (dot DotCall) compile2(vm *Machine) int {
	/* pos := len(cs.output)
	dot.compile(vm)
	return pos */

	panic("implement")
}
