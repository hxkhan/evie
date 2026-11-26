package ast

import (
	"fmt"
	"github.com/hxkhan/evie/diag"
	"strings"
)

// Fn is a function declaration, or a function expression when UsedAsExpr is set;
// with IsVariadic the last of Args collects any further arguments
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

// Go is `go call(...)`, which runs the call on a new fiber and gives back its task
type Go struct {
	diag.Pos
	Fn Node
}

// Call is a call of Fn with Args
type Call struct {
	diag.Pos
	Fn   Node
	Args []Node
}

// Return is a return statement; a bare `return` returns nil
type Return struct {
	diag.Pos
	Value Node
}

// Await is `await task`, which waits for the task and gives back its result
type Await struct {
	diag.Pos
	Task Node
}

// AwaitAll is `await.all(...)`, which waits for every task
type AwaitAll struct {
	diag.Pos
	Tasks []Node
}

// AwaitAny is `await.any(...)`, which waits for the first task to succeed
type AwaitAny struct {
	diag.Pos
	Tasks []Node
}

// AwaitRace is `await.race(...)`, which waits for the first task to finish
type AwaitRace struct {
	diag.Pos
	Tasks []Node
}

// AwaitTimeout is `await.timeout(task, ms)`, which waits for the task for at most
// Timeout milliseconds
type AwaitTimeout struct {
	diag.Pos
	Task    Node
	Timeout Node
}

// String returns the function as Evie source
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

// String returns the call as Evie source
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

// String returns the statement as Evie source
func (ret Return) String() string {
	return fmt.Sprintf("return %v", ret.Value)
}
