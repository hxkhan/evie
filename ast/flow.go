package ast

import "github.com/hxkhan/evie/token"

type Conditional struct {
	token.Pos
	Condition Node // [required]
	Action    Node // [required]
	Otherwise Node // [optional]
}
