package ast

import "github.com/hxkhan/evie/token"

type Node interface {
	Line() int
}

type Package struct {
	token.Pos
	Name    string
	Imports []string
	Code    []Node
}

type Literal interface {
	bool | float64 | string | struct{}
}

type Input[T Literal] struct {
	token.Pos
	Value T
}

type Block struct {
	token.Pos
	Code []Node
}

type Echo struct {
	token.Pos
	Value Node
}
