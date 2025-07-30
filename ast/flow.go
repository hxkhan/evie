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

func (node Conditional) String() string {
	if node.Action == nil {
		return fmt.Sprintf("if (%v)", node.Condition)
	} else if node.Otherwise == nil {
		return fmt.Sprintf("if (%v) %v", node.Condition, node.Action)
	}
	return fmt.Sprintf("if (%v) %v else %v", node.Condition, node.Action, node.Otherwise)
}
