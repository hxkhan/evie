package ast

import "github.com/hxkhan/evie/token"

type Fn struct {
	token.Pos
	Name   string
	Args   []string
	Action Node
}

type Go struct {
	token.Pos
	Fn Node
}

type Call struct {
	token.Pos
	Fn   Node
	Args []Node
}

type Return struct {
	token.Pos
	Value Node
}

type Await struct {
	token.Pos
	Task Node
}

type AwaitAll struct {
	token.Pos
	Names []string
}

type AwaitAny struct {
	token.Pos
	Names []string
}

type FieldAccess struct {
	token.Pos
	Left  Node
	Right string
}

type DotCall struct {
	token.Pos
	Left  Node
	Right Node
	Args  []Node
}
