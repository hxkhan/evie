package ast

import (
	"fmt"

	"github.com/hxkhan/evie/token"
)

// fn(a,b,c...) -> d
// fn(int|float) -> int

type FnType struct {
	token.Pos
	Params  []Node
	Returns Node
}

func (fn FnType) String() string {
	return fmt.Sprintf("fn(%v) -> %v", fn.Params, fn.Returns)
}

type UnionType struct {
	token.Pos
	Lhs Node
	Rhs Node
}

func (ut UnionType) String() string {
	return fmt.Sprintf("%v | %v", ut.Lhs, ut.Rhs)
}
