package ast

import (
	"fmt"
	"strings"

	"github.com/hxkhan/evie/token"
)

type SyncMode int

// Decide will decide if to make user fn synced or unsynced based on the sync mode of the environment
func (sm SyncMode) Decide(currently bool) (after bool) {
	switch sm {
	case SyncedMode:
		return true
	case UnsyncedMode:
		return false
	}
	return currently
}

const (
	UndefinedMode SyncMode = iota
	SyncedMode
	UnsyncedMode
)

type Fn struct {
	token.Pos
	Name       string
	Args       []string
	SyncMode   SyncMode
	Action     Node
	IsPublic   bool
	UsedAsExpr bool
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

type FieldAccess struct {
	token.Pos
	Lhs Node
	Rhs string
}

/* func (node FieldAccess) String() string {
	return fmt.Sprintf("%v.%v", node.Lhs, node.Rhs)
} */

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

func (fa FieldAccess) String() string {
	return fmt.Sprintf("%v.%s", fa.Lhs, fa.Rhs)
}
