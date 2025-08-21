package ast

import (
	"fmt"

	"hxkhan.dev/evie/token"
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

type Unsynced struct {
	token.Pos
	Action Node // [required]
}

type Synced struct {
	token.Pos
	Action Node // [required]
}

type Continue struct {
	token.Pos
}

type Break struct {
	token.Pos
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

func (node Continue) String() string {
	return "continue"
}

func (node Break) String() string {
	return "break"
}
