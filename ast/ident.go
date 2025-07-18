package ast

import "github.com/hxkhan/evie/token"

type IdentDec struct {
	token.Pos
	Name  string
	Value Node
}

type IdentGet struct {
	token.Pos
	Name string
}

type IdentSet struct {
	token.Pos
	Name  string
	Value Node
}
