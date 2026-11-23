package ast

import (
	"fmt"
	"github.com/hxkhan/evie/diag"
	"strings"
)

type Fn struct {
	diag.Pos
	Name       string
	NamePos    diag.Pos // where the name is written; unknown for an unnamed fn
	Args       []string
	ArgPos     []diag.Pos // where each of Args is written
	IsVariadic bool
	Action     Node
	UsedAsExpr bool // if true then create and return, else declare
}

type Go struct {
	diag.Pos
	Fn Node
}

type Call struct {
	diag.Pos
	Fn   Node
	Args []Node
}

type Return struct {
	diag.Pos
	Value Node
}

type Await struct {
	diag.Pos
	Task Node
}

type AwaitAll struct {
	diag.Pos
	Tasks []Node
}

type AwaitAny struct {
	diag.Pos
	Tasks []Node
}

type AwaitRace struct {
	diag.Pos
	Tasks []Node
}

type AwaitTimeout struct {
	diag.Pos
	Task    Node
	Timeout Node
}

func (fn Fn) String() string {
	b := strings.Builder{}
	b.WriteString("fn")

	if fn.Name != "" {
		b.WriteByte(' ')
		b.WriteString(fn.Name)
	}

	// args
	b.WriteByte('(')
	for i, name := range fn.Args {
		if fn.IsVariadic && i == len(fn.Args)-1 {
			b.WriteString("...")
		}
		b.WriteString(name)
		if i != len(fn.Args)-1 {
			b.WriteByte(',')
		}
	}
	b.WriteByte(')')

	b.WriteString(fmt.Sprint(fn.Action))

	return b.String()
}

func (call Call) String() string {
	b := strings.Builder{}
	b.WriteString(fmt.Sprint(call.Fn))
	b.WriteByte('(')

	for i, arg := range call.Args {
		b.WriteString(fmt.Sprint(arg))
		if i != len(call.Args)-1 {
			b.WriteByte(',')
		}
	}

	b.WriteByte(')')
	return b.String()
}

func (ret Return) String() string {
	return fmt.Sprintf("return %v", ret.Value)
}
