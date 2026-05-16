package ast

import (
	"fmt"
	"strings"

	"github.com/hxkhan/evie/token"
)

type SyncMode int

func (sm SyncMode) String() string {
	switch sm {
	case UndefinedMode:
		return "UndefinedMode"
	case SyncedMode:
		return "SyncedMode"
	}

	return "UnknownMode"
}

const (
	// Undefined mode inherits from lexical parent
	UndefinedMode SyncMode = iota
	// Synced mode assumes GIL
	SyncedMode
)

// Param represents a single typed function argument
type Param struct {
	Name string
	Type Node
}

type Fn struct {
	token.Pos
	Name       string
	Params     []Param
	ReturnType Node
	SyncMode   SyncMode
	Action     Node
	IsPublic   bool
	UsedAsExpr bool // if true then create and return, else declare
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
	Tasks []Node
}

type AwaitAny struct {
	token.Pos
	Tasks []Node
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
	for i, param := range fn.Params {
		b.WriteString(param.Name)
		b.WriteByte(':')
		b.WriteByte(' ')
		b.WriteString(param.Type.String())
		if i != len(fn.Params)-1 {
			b.WriteByte(',')
		}
	}
	b.WriteByte(')')

	if fn.ReturnType != nil {
		b.WriteByte(':')
		b.WriteString(fn.ReturnType.String())
		b.WriteByte(' ')
	}

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

func (node Go) String() string {
	return "go " + node.Fn.String()
}

func (node Await) String() string {
	return "await " + node.Task.String()
}

func (node AwaitAll) String() string {
	return fmt.Sprintf("await.all(%v)", node.Tasks)
}

func (node AwaitAny) String() string {
	return fmt.Sprintf("await.any(%v)", node.Tasks)
}
