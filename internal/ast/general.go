package ast

import (
	"github.com/hk-32/evie/internal/op"
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
	compile(cs *CompilerState) (pos int)
}

type Input struct {
	Value any
}

func (in Input) compile(cs *CompilerState) int {
	switch v := in.Value.(type) {
	case nil:
		return cs.emit(op.NULL)
	case string:
		return cs.emitString(v)
	case int64:
		return cs.emitInt64(v)
	case float64:
		return cs.emitFloat64(v)
	case bool:
		if v {
			return cs.emit(op.TRUE)
		} else {
			return cs.emit(op.FALSE)
		}
	case int: // convenience
		return cs.emitInt64(int64(v))
	}

	panic("Input.compile -> unimplemented type")
}

type Block []Node

func (b Block) compile(cs *CompilerState) int {
	pos := cs.len()
	for _, node := range b {
		node.compile(cs)
	}
	return pos
}

type Echo struct {
	Value Node
}

func (out Echo) compile(cs *CompilerState) int {
	pos := cs.emit(op.ECHO)
	out.Value.compile(cs)

	return pos
}
