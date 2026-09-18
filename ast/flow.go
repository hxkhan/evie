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

type For struct {
	token.Pos
	IndexVar   string // optional, "" if none
	ItemVar    string // [required]
	Collection Node   // [required]
	Action     Node   // [required]
}

type Unsynced struct {
	token.Pos
	Action Node // [required]
}

type Synced struct {
	token.Pos
	Action Node // [required]
}

type Catch struct {
	token.Pos
	Action Node // [required]
}

type Continue struct {
	token.Pos
}

type Break struct {
	token.Pos
}

type Throw struct {
	token.Pos
	Value Node
}

type SwitchCase struct {
	token.Pos
	Values []Node // multiple case expressions e.g. case 1, 2, 3:
	Action Node   // body of the case (ast.Block)
}

type Switch struct {
	token.Pos
	Value   Node         // [optional] value to match against; nil for tagless switch
	Cases   []SwitchCase // list of case branches
	Default Node         // [optional] default action (ast.Block), nil if none
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

func (node For) String() string {
	if node.IndexVar != "" {
		return fmt.Sprintf("for %v, %v in %v %v", node.IndexVar, node.ItemVar, node.Collection, node.Action)
	}
	return fmt.Sprintf("for %v in %v %v", node.ItemVar, node.Collection, node.Action)
}

func (node Continue) String() string {
	return "continue"
}

func (node Break) String() string {
	return "break"
}

func (node Throw) String() string {
	return fmt.Sprintf("throw %v", node.Value)
}

func (node Switch) String() string {
	if node.Value != nil {
		return fmt.Sprintf("switch %v { ... }", node.Value)
	}
	return "switch { ... }"
}
