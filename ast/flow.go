package ast

import (
	"fmt"

	"github.com/hxkhan/evie/token"
)

type Conditional struct {
	token.Pos
	Condition Node // [required]
	Action    Node // [required]
	Otherwise Node // [optional]
}

type While struct {
	token.Pos
	Condition Node // [required]
	Action    Node // [required]
}

func (node Conditional) String() string {
	if node.Action == nil {
		return fmt.Sprintf("if (%v)", node.Condition)
	} else if node.Otherwise == nil {
		return fmt.Sprintf("if (%v) %v", node.Condition, node.Action)
	}
	return fmt.Sprintf("if (%v) %v else %v", node.Condition, node.Action, node.Otherwise)
}

func (node While) String() string {
	return fmt.Sprintf("while (%v) %v", node.Condition, node.Action)
}
