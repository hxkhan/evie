package ast

import (
	"fmt"
	"github.com/hxkhan/evie/diag"
)

// Conditional is an if statement, with an optional else branch in Otherwise
type Conditional struct {
	diag.Pos
	Condition Node // [required]
	Action    Node // [required]
	Otherwise Node // [optional]
}

// While is a while loop
type While struct {
	diag.Pos
	Condition Node // [required]
	Action    Node // [required]
}

// For is `for item in collection` or `for index, item in collection`
type For struct {
	diag.Pos
	IndexVar   string   // optional, "" if none
	IndexPos   diag.Pos // where IndexVar is written, when there is one
	ItemVar    string   // [required]
	ItemPos    diag.Pos // where ItemVar is written
	Collection Node     // [required]
	Action     Node     // [required]
}

// Catch is `catch(expr)` or `catch { ... }`: it gives back the value of
// Action, or the error raised while running it
type Catch struct {
	diag.Pos
	Action Node // [required]
}

// Continue is a continue statement
type Continue struct {
	diag.Pos
}

// Break is a break statement
type Break struct {
	diag.Pos
}

// Throw is a throw statement; a bare `throw` throws the string "error"
type Throw struct {
	diag.Pos
	Value Node
}

// SwitchCase is one `case a, b:` branch of a Switch
type SwitchCase struct {
	diag.Pos
	Values []Node // multiple case expressions e.g. case 1, 2, 3:
	Action Node   // body of the case (ast.Block)
}

// Switch is a switch statement, matching Value against each case in order,
// or testing each case as a condition when Value is nil
type Switch struct {
	diag.Pos
	Value   Node         // [optional] value to match against; nil for tagless switch
	Cases   []SwitchCase // list of case branches
	Default Node         // [optional] default action (ast.Block), nil if none
	End     diag.Pos     // the closing '}'
}

// String returns the statement as Evie source
func (node Conditional) String() string {
	if node.Action == nil {
		return fmt.Sprintf("if (%v)", node.Condition)
	} else if node.Otherwise == nil {
		return fmt.Sprintf("if (%v) %v", node.Condition, node.Action)
	}
	return fmt.Sprintf("if (%v) %v else %v", node.Condition, node.Action, node.Otherwise)
}

// String returns the loop as Evie source
func (node While) String() string {
	return fmt.Sprintf("while (%v) %v", node.Condition, node.Action)
}

// String returns the loop as Evie source
func (node For) String() string {
	if node.IndexVar != "" {
		return fmt.Sprintf("for %v, %v in %v %v", node.IndexVar, node.ItemVar, node.Collection, node.Action)
	}
	return fmt.Sprintf("for %v in %v %v", node.ItemVar, node.Collection, node.Action)
}

// String returns "continue"
func (node Continue) String() string {
	return "continue"
}

// String returns "break"
func (node Break) String() string {
	return "break"
}

// String returns the statement as Evie source
func (node Throw) String() string {
	return fmt.Sprintf("throw %v", node.Value)
}

// String returns the switch's head, with its cases elided
func (node Switch) String() string {
	if node.Value != nil {
		return fmt.Sprintf("switch %v { ... }", node.Value)
	}
	return "switch { ... }"
}
