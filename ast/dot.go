package ast

import (
	"github.com/hk-32/evie/op"
)

type DotCall struct {
	Left  Node
	Right Node
	Args  []Node
}

func (dot DotCall) compile(cs *CompilerState) int {
	// namespaces e.g. json.decode(...)
	iGetLeft, isLeftIdentGet := dot.Left.(IdentGet)
	iGetRight, isRightIdentGet := dot.Right.(IdentGet)
	if isLeftIdentGet && isRightIdentGet {
		name := iGetLeft.Name + "." + iGetRight.Name

		if cs.isInBuiltIn(name) {
			pos := cs.emit(op.CALL, byte(len(dot.Args)))
			IdentGet{name}.compile(cs)

			for _, arg := range dot.Args {
				arg.compile(cs)
			}
			return pos
		}
	}

	// method calls
	pos := cs.emit(op.CALL, byte(len(dot.Args)+1))
	dot.Right.compile(cs) // iGet function

	dot.Left.compile(cs) // iGet
	for _, arg := range dot.Args {
		arg.compile(cs)
	}

	return pos
}

func (dot DotCall) compile2(cs *CompilerState) int {
	pos := len(cs.output)
	dot.compile(cs)
	return pos
}
