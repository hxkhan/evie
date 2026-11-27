//go:build ignore

// gen_ops writes ops_gen.go: the operator instructions specialised for every
// combination of operator and operand shape.
//
// An operator applied to two locals, to a local and a constant, or to a
// constant and a local is the hottest code the interpreter runs, and Go only
// inlines Value.sub and its siblings when they are called directly from the
// closure. Calling them through a table costs 10% to 40% on numeric loops, so
// the closures are spelled out, one per operator and shape; this program
// spells them out so that nobody has to maintain the copies by hand. Value.add,
// Value.mod and Value.Equals are too big to inline at all, so for +, %, == and
// != the closures compare numbers themselves before calling the method.
//
// Only locals in stack slots get shapes of their own. A captured or boxed local
// is read through its own instruction like any other expression: one call more,
// for code that is rarely the hottest, instead of a second set of shapes.
//
//	go generate ./vm
package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"strings"
)

// op is one binary operator: its AST name, its symbol, and how a closure
// applies it to a and b
type op struct {
	Name   string // the ast.Operator constant
	Symbol string
	Method string // the Value method for method operators
	Kind   string // method, eq, noteq, or, and
}

var ops = []op{
	{"AddOp", "+", "add", "method"},
	{"SubOp", "-", "sub", "method"},
	{"MulOp", "*", "mul", "method"},
	{"DivOp", "/", "div", "method"},
	{"ModOp", "%", "mod", "method"},
	{"EqOp", "==", "", "eq"},
	{"NotEqOp", "!=", "", "noteq"},
	{"LtOp", "<", "lessThan", "method"},
	{"GtOp", ">", "greaterThan", "method"},
	{"LtEqOp", "<=", "lessThanOrEqualTo", "method"},
	{"GtEqOp", ">=", "greaterThanOrEqualTo", "method"},
	{"OrOp", "||", "", "or"},
	{"AndOp", "&&", "", "and"},
}

func (o op) arithmetic() bool { return o.Kind == "method" && strings.ContainsAny(o.Symbol, "+-*/%") }
func (o op) comparison() bool {
	return o.Kind == "eq" || o.Kind == "noteq" || (o.Kind == "method" && !o.arithmetic())
}

// result is the body that turns the operands named a and b into the
// instruction's result
func (o op) result(a, b string) string {
	switch o.Kind {
	case "eq", "noteq":
		cmp, not := "==", ""
		if o.Kind == "noteq" {
			cmp, not = "!=", "!"
		}
		return fmt.Sprintf(`if %[1]s.pointer == f64Type && %[2]s.pointer == f64Type {
		return BoxBool(math.Float64frombits(%[1]s.scalar) %[3]s math.Float64frombits(%[2]s.scalar)), nil
	}
	return BoxBool(%[4]s%[1]s.equalsSlow(%[2]s)), nil`, a, b, cmp, not)
	case "or":
		return fmt.Sprintf("return BoxBool(%s.IsTruthy() || %s.IsTruthy()), nil", a, b)
	case "and":
		return fmt.Sprintf("return BoxBool(%s.IsTruthy() && %s.IsTruthy()), nil", a, b)
	}
	fallback := fmt.Sprintf("return Value{}, operatorError(site, %q, %s, %s)", o.Symbol, a, b)
	if o.comparison() {
		// strings are ordered too, out of line so the number path stays small
		fallback = fmt.Sprintf("return orderSlow(site, ast.%s, %s, %s)", o.Name, a, b)
	}
	return o.numeric(a, b, func(v string) string { return "return " + v + ", nil" }) + fmt.Sprintf(`if result, ok := %[1]s.%[3]s(%[2]s); ok {
		return result, nil
	}
	%[4]s`, a, b, o.Method, fallback)
}

// numeric is the number fast path of + and %, whose methods are too big for Go
// to inline: when a and b are both numbers it does done with the result. The
// other arithmetic methods inline, so they need none.
func (o op) numeric(a, b string, done func(v string) string) string {
	fa, fb := "math.Float64frombits("+a+".scalar)", "math.Float64frombits("+b+".scalar)"
	var v string
	switch o.Method {
	case "add":
		v = "BoxNumber(" + fa + " + " + fb + ")"
	case "mod":
		v = "modNumbers(" + fa + ", " + fb + ")"
	default:
		return ""
	}
	return fmt.Sprintf("if %s.pointer == f64Type && %s.pointer == f64Type {\n\t\t%s\n\t}\n\t", a, b, done(v))
}

// store is the body that applies an arithmetic operator to a and b and writes
// the result to dst
func (o op) store(a, b, dst string) string {
	return o.numeric(a, b, func(v string) string { return dst + " = " + v + "\n\t\treturn Value{}, nil" }) + fmt.Sprintf(`if result, ok := %[1]s.%[3]s(%[2]s); ok {
		%[5]s = result
		return Value{}, nil
	}
	return Value{}, operatorError(site, %[4]q, %[1]s, %[2]s)`, a, b, o.Method, o.Symbol, dst)
}

// update is the body that applies the operator to *cell and the operand named
// b in place
func (o op) update(b string) string {
	return o.numeric("cell", b, func(v string) string { return "*cell = " + v + "\n\t\treturn Value{}, nil" }) + fmt.Sprintf(`if result, ok := cell.%[2]s(%[1]s); ok {
		*cell = result
		return Value{}, nil
	}
	return Value{}, operatorError(site, %[3]q, *cell, %[1]s)`, b, o.Method, o.Symbol)
}

// condition is the body that decides a comparison: numbers is the test that
// both operands are numbers and bn the numeric form of b
func (o op) condition(numbers string, bn string) string {
	switch o.Kind {
	case "eq":
		return fmt.Sprintf("if %s {\n\t\treturn math.Float64frombits(a.scalar) == %s, nil\n\t}\n\treturn a.equalsSlow(b), nil", numbers, bn)
	case "noteq":
		return fmt.Sprintf("if %s {\n\t\treturn math.Float64frombits(a.scalar) != %s, nil\n\t}\n\treturn !a.equalsSlow(b), nil", numbers, bn)
	}
	return fmt.Sprintf(`if %s {
		return math.Float64frombits(a.scalar) %s %s, nil
	}
	res, ok := a.%s(b)
	if !ok {
		return orderTest(site, ast.%s, a, b)
	}
	return res.IsTruthy(), nil`, numbers, o.Symbol, bn, o.Method, o.Name)
}

func main() {
	var b bytes.Buffer
	p := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }

	p(`// Code generated by gen_ops.go; DO NOT EDIT.

package vm

import (
	"math"

	"github.com/hxkhan/evie/ast"
)

// emitBinOp compiles a binary operator. Operands that are locals or constants
// are read without evaluating an instruction, and numbers are handled in the
// closure itself or by a method small enough to inline.
func (vm *Instance) emitBinOp(node ast.BinOp) instruction {
	site := vm.at(node)
	if val, ok := vm.evaluate(node).(Value); ok {
		return func(fbr *fiber) (Value, exception) {
			return val, nil
		}
	}

	lhs, rhs := vm.evaluate(node.Lhs), vm.evaluate(node.Rhs)

	if l, ok := lhs.(Value); ok {
		if r, ok := rhs.(local); ok {
			if !r.isCaptured && !r.isBoxed {
				ri := int(r.index)
				switch node.Operator {
`)
	for _, o := range ops {
		p("case ast.%s:\n\treturn func(fbr *fiber) (Value, exception) {\n\t\tb := fbr.stack[fbr.base+ri]\n\t\t%s\n\t}\n", o.Name, o.result("l", "b"))
	}
	p(`				}
			}
			`)
	p(`		}
	}

	if l, ok := lhs.(local); ok {
		if r, ok := rhs.(local); ok {
			if !l.isCaptured && !l.isBoxed && !r.isCaptured && !r.isBoxed {
				li, ri := int(l.index), int(r.index)
				switch node.Operator {
`)
	for _, o := range ops {
		p("case ast.%s:\n\treturn func(fbr *fiber) (Value, exception) {\n\t\ts := fbr.stack[fbr.base:]\n\t\ta, b := s[li], s[ri]\n\t\t%s\n\t}\n", o.Name, o.result("a", "b"))
	}
	p(`				}
			}
			`)
	p(`		}
		if r, ok := rhs.(Value); ok {
			if !l.isCaptured && !l.isBoxed {
				li := int(l.index)
				switch node.Operator {
`)
	for _, o := range ops {
		p("case ast.%s:\n\treturn func(fbr *fiber) (Value, exception) {\n\t\ta := fbr.stack[fbr.base+li]\n\t\t%s\n\t}\n", o.Name, o.result("a", "r"))
	}
	p(`				}
			}
			`)
	p(`		}
	}

	// Single-sided operand specialization for non-short-circuit operators:
	// eliminates one indirect closure call in chained expressions such as
	// (xx - yy) + x0, (2.0 * x) * y + y0, or (py / size) * 2.0 - 1.0.
	if node.Operator != ast.AndOp && node.Operator != ast.OrOp {
		if r, ok := rhs.(local); ok {
			left := vm.compile(node.Lhs)
			if !r.isCaptured && !r.isBoxed {
				ri := int(r.index)
				switch node.Operator {
`)
	for _, o := range ops {
		if o.Kind == "and" || o.Kind == "or" {
			continue
		}
		p("case ast.%s:\n\treturn func(fbr *fiber) (Value, exception) {\n\t\ta, exc := left(fbr)\n\t\tif exc != nil {\n\t\t\treturn a, exc\n\t\t}\n\t\tb := fbr.stack[fbr.base+ri]\n\t\t%s\n\t}\n", o.Name, o.result("a", "b"))
	}
	p(`				}
			}
			`)
	p(`		}
		if r, ok := rhs.(Value); ok {
			left := vm.compile(node.Lhs)
			switch node.Operator {
`)
	for _, o := range ops {
		if o.Kind == "and" || o.Kind == "or" {
			continue
		}
		p("case ast.%s:\n\treturn func(fbr *fiber) (Value, exception) {\n\t\ta, exc := left(fbr)\n\t\tif exc != nil {\n\t\t\treturn a, exc\n\t\t}\n\t\t%s\n\t}\n", o.Name, o.result("a", "r"))
	}
	p(`			}
		}
		if l, ok := lhs.(local); ok {
			right := vm.compile(node.Rhs)
			if !l.isCaptured && !l.isBoxed {
				li := int(l.index)
				switch node.Operator {
`)
	for _, o := range ops {
		if o.Kind == "and" || o.Kind == "or" {
			continue
		}
		p("case ast.%s:\n\treturn func(fbr *fiber) (Value, exception) {\n\t\ta := fbr.stack[fbr.base+li]\n\t\tb, exc := right(fbr)\n\t\tif exc != nil {\n\t\t\treturn b, exc\n\t\t}\n\t\t%s\n\t}\n", o.Name, o.result("a", "b"))
	}
	p(`				}
			}
			`)
	p(`		}
		if l, ok := lhs.(Value); ok {
			right := vm.compile(node.Rhs)
			switch node.Operator {
`)
	for _, o := range ops {
		if o.Kind == "and" || o.Kind == "or" {
			continue
		}
		p("case ast.%s:\n\treturn func(fbr *fiber) (Value, exception) {\n\t\tb, exc := right(fbr)\n\t\tif exc != nil {\n\t\t\treturn b, exc\n\t\t}\n\t\t%s\n\t}\n", o.Name, o.result("l", "b"))
	}
	p(`			}
		}
	}
	// generic compilation; && and || evaluate the right side only when they must
	left, right := vm.compile(node.Lhs), vm.compile(node.Rhs)
	switch node.Operator {
`)
	for _, o := range ops {
		switch o.Kind {
		case "or":
			p(`case ast.OrOp:
	return func(fbr *fiber) (Value, exception) {
		a, exc := left(fbr)
		if exc != nil {
			return a, exc
		}
		if a.IsTruthy() {
			return BoxBool(true), nil
		}
		b, exc := right(fbr)
		if exc != nil {
			return b, exc
		}
		return BoxBool(b.IsTruthy()), nil
	}
`)
		case "and":
			p(`case ast.AndOp:
	return func(fbr *fiber) (Value, exception) {
		a, exc := left(fbr)
		if exc != nil {
			return a, exc
		}
		if !a.IsTruthy() {
			return BoxBool(false), nil
		}
		b, exc := right(fbr)
		if exc != nil {
			return b, exc
		}
		return BoxBool(b.IsTruthy()), nil
	}
`)
		default:
			p("case ast.%s:\n\treturn func(fbr *fiber) (Value, exception) {\n\t\ta, exc := left(fbr)\n\t\tif exc != nil {\n\t\t\treturn a, exc\n\t\t}\n\t\tb, exc := right(fbr)\n\t\tif exc != nil {\n\t\t\treturn b, exc\n\t\t}\n\t\t%s\n\t}\n", o.Name, o.result("a", "b"))
		}
	}
	p(`	}
	panic(vm.compileErrorAt(node, "unsupported operator '%%v'", node.Operator))
}

// emitMutableBinOp compiles a compound assignment such as x += 1 on a local
// or a global; fields and subscripts have their own emitters.
func (vm *Instance) emitMutableBinOp(node ast.MutableBinOp) instruction {
	site := vm.at(node)
	switch target := node.Lhs.(type) {
	case ast.FieldAccess:
		return vm.emitCompoundFieldAssign(node, target)
	case ast.Subscript:
		return vm.emitCompoundSubscriptAssign(node, target)
	}

	switch lhs := vm.assignee(node.Lhs).(type) {
	case local:
		if lhs.isStatic {
			panic(vm.compileErrorAt(node.Lhs, "cannot assign to a constant").withHint("declare it with var to allow reassignment"))
		}
		switch rhs := vm.evaluate(node.Rhs).(type) {
		case local:
			if !lhs.isCaptured && !lhs.isBoxed && !rhs.isCaptured && !rhs.isBoxed {
				li, ri := int(lhs.index), int(rhs.index)
				switch node.Operator {
`)
	for _, o := range ops {
		if !o.arithmetic() {
			continue
		}
		p("case ast.%s:\n\treturn func(fbr *fiber) (Value, exception) {\n\t\ts := fbr.stack[fbr.base:]\n\t\tcell, b := &s[li], s[ri]\n\t\t%s\n\t}\n", o.Name, o.update("b"))
	}
	p(`				}
			}
			`)
	p(`		case Value:
			if !lhs.isCaptured && !lhs.isBoxed {
				li := int(lhs.index)
				switch node.Operator {
`)
	for _, o := range ops {
		if !o.arithmetic() {
			continue
		}
		p("case ast.%s:\n\treturn func(fbr *fiber) (Value, exception) {\n\t\tcell := &fbr.stack[fbr.base+li]\n\t\t%s\n\t}\n", o.Name, o.update("rhs"))
	}
	p(`				}
			}
			`)
	p(`		}
		rhs := vm.compile(node.Rhs)
		if !lhs.isCaptured && !lhs.isBoxed {
			li := int(lhs.index)
			switch node.Operator {
`)
	for _, o := range ops {
		if !o.arithmetic() {
			continue
		}
		p("case ast.%s:\n\treturn func(fbr *fiber) (Value, exception) {\n\t\tb, exc := rhs(fbr)\n\t\tif exc != nil {\n\t\t\treturn b, exc\n\t\t}\n\t\tcell := &fbr.stack[fbr.base+li]\n\t\t%s\n\t}\n", o.Name, o.update("b"))
	}
	p(`			}
		}
		switch node.Operator {
`)
	for _, o := range ops {
		if !o.arithmetic() {
			continue
		}
		p("case ast.%s:\n\treturn func(fbr *fiber) (Value, exception) {\n\t\tb, exc := rhs(fbr)\n\t\tif exc != nil {\n\t\t\treturn b, exc\n\t\t}\n\t\tcell := fbr.get(lhs)\n\t\t%s\n\t}\n", o.Name, o.update("b"))
	}
	p(`		}

	case pkgGlobal:
		if lhs.IsStatic {
			panic(vm.compileErrorAt(node.Lhs, "cannot assign to a constant").withHint("declare it with var to allow reassignment"))
		}
		cell := lhs.Value
		rhs := vm.compile(node.Rhs)
		switch node.Operator {
`)
	for _, o := range ops {
		if !o.arithmetic() {
			continue
		}
		p("case ast.%s:\n\treturn func(fbr *fiber) (Value, exception) {\n\t\tb, exc := rhs(fbr)\n\t\tif exc != nil {\n\t\t\treturn b, exc\n\t\t}\n\t\t%s\n\t}\n", o.Name, o.update("b"))
	}
	p(`		}
	}
	panic(vm.compileErrorAt(node, "unsupported compound operator '%%v'", node.Operator))
}

// compileLocalBinAssign fuses a local declaration or assignment whose RHS is an
// arithmetic binary op ('dest := a op b', 'dest = expr op c') directly into
// the destination slot, skipping one indirect closure dispatch per statement.
func (vm *Instance) compileLocalBinAssign(dest local, bin ast.BinOp) (instruction, bool) {
	if dest.isCaptured || dest.isBoxed {
		return nil, false
	}
	di := int(dest.index)
	site := vm.at(bin)
	lhs, rhs := vm.evaluate(bin.Lhs), vm.evaluate(bin.Rhs)

	if l, ok := lhs.(local); ok && !l.isCaptured && !l.isBoxed {
		li := int(l.index)
		if r, ok := rhs.(local); ok && !r.isCaptured && !r.isBoxed {
			ri := int(r.index)
			switch bin.Operator {
`)
	for _, o := range ops {
		if !o.arithmetic() {
			continue
		}
		p(`case ast.%s:
	return func(fbr *fiber) (Value, exception) {
		s := fbr.stack[fbr.base:]
		a, b := s[li], s[ri]
		%s
	}, true
`, o.Name, o.store("a", "b", "s[di]"))
	}
	p(`			}
		}
		if r, ok := rhs.(Value); ok {
			switch bin.Operator {
`)
	for _, o := range ops {
		if !o.arithmetic() {
			continue
		}
		p(`case ast.%s:
	return func(fbr *fiber) (Value, exception) {
		s := fbr.stack[fbr.base:]
		a := s[li]
		%s
	}, true
`, o.Name, o.store("a", "r", "s[di]"))
	}
	p(`			}
		}
	}
	if l, ok := lhs.(Value); ok {
		if r, ok := rhs.(local); ok && !r.isCaptured && !r.isBoxed {
			ri := int(r.index)
			switch bin.Operator {
`)
	for _, o := range ops {
		if !o.arithmetic() {
			continue
		}
		p(`case ast.%s:
	return func(fbr *fiber) (Value, exception) {
		s := fbr.stack[fbr.base:]
		b := s[ri]
		%s
	}, true
`, o.Name, o.store("l", "b", "s[di]"))
	}
	p(`			}
		}
	}
	if r, ok := rhs.(local); ok && !r.isCaptured && !r.isBoxed {
		ri := int(r.index)
		switch bin.Operator {
`)
	for _, o := range ops {
		if !o.arithmetic() {
			continue
		}
		p(`case ast.%s:
	left := vm.compile(bin.Lhs)
	return func(fbr *fiber) (Value, exception) {
		a, exc := left(fbr)
		if exc != nil {
			return a, exc
		}
		s := fbr.stack[fbr.base:]
		b := s[ri]
		%s
	}, true
`, o.Name, o.store("a", "b", "s[di]"))
	}
	p(`		}
	}
	if r, ok := rhs.(Value); ok {
		switch bin.Operator {
`)
	for _, o := range ops {
		if !o.arithmetic() {
			continue
		}
		p(`case ast.%s:
	left := vm.compile(bin.Lhs)
	return func(fbr *fiber) (Value, exception) {
		a, exc := left(fbr)
		if exc != nil {
			return a, exc
		}
		%s
	}, true
`, o.Name, o.store("a", "r", "fbr.stack[fbr.base+di]"))
	}
	p(`		}
	}
	return nil, false
}

// compileComparison compiles a comparison used as a condition straight to a
// Go bool when its operands are locals or constants (or an expression compared
// against a local or constant), comparing numbers without boxing.
func (vm *Instance) compileComparison(bin ast.BinOp, site site) (func(fbr *fiber) (bool, exception), bool) {
	if l, isLocal := vm.evaluate(bin.Lhs).(local); isLocal {
		switch r := vm.evaluate(bin.Rhs).(type) {
		case Value:
			b := r
			if bn, isNum := r.AsFloat64(); isNum {
				if !l.isCaptured && !l.isBoxed {
					li := int(l.index)
					switch bin.Operator {
`)
	for _, o := range ops {
		if !o.comparison() {
			continue
		}
		p("case ast.%s:\n\treturn func(fbr *fiber) (bool, exception) {\n\t\ta := fbr.stack[fbr.base+li]\n\t\t%s\n\t}, true\n", o.Name, o.condition("a.pointer == f64Type", "bn"))
	}
	p(`					}
				}
				`)
	p(`			}
			switch bin.Operator {
			case ast.EqOp:
				return func(fbr *fiber) (bool, exception) {
					return fbr.get(l).Equals(b), nil
				}, true
			case ast.NotEqOp:
				return func(fbr *fiber) (bool, exception) {
					return !fbr.get(l).Equals(b), nil
				}, true
			}
		case local:
			if !l.isCaptured && !l.isBoxed && !r.isCaptured && !r.isBoxed {
				li, ri := int(l.index), int(r.index)
				switch bin.Operator {
`)
	for _, o := range ops {
		if !o.comparison() {
			continue
		}
		p("case ast.%s:\n\treturn func(fbr *fiber) (bool, exception) {\n\t\ts := fbr.stack[fbr.base:]\n\t\ta, b := s[li], s[ri]\n\t\t%s\n\t}, true\n", o.Name, o.condition("a.pointer == f64Type && b.pointer == f64Type", "math.Float64frombits(b.scalar)"))
	}
	p(`				}
			}
			`)
	p(`		}
		return nil, false
	}

	if !vm.cp.inline {
		return nil, false
	}
	switch r := vm.evaluate(bin.Rhs).(type) {
	case Value:
		b := r
		if bn, isNum := r.AsFloat64(); isNum {
			switch bin.Operator {
`)
	for _, o := range ops {
		if !o.comparison() {
			continue
		}
		p("case ast.%s:\n\tleft := vm.compile(bin.Lhs)\n\treturn func(fbr *fiber) (bool, exception) {\n\t\ta, exc := left(fbr)\n\t\tif exc != nil {\n\t\t\treturn false, exc\n\t\t}\n\t\t%s\n\t}, true\n", o.Name, o.condition("a.pointer == f64Type", "bn"))
	}
	p(`			}
		}
		switch bin.Operator {
		case ast.EqOp:
			left := vm.compile(bin.Lhs)
			return func(fbr *fiber) (bool, exception) {
				a, exc := left(fbr)
				if exc != nil {
					return false, exc
				}
				return a.Equals(b), nil
			}, true
		case ast.NotEqOp:
			left := vm.compile(bin.Lhs)
			return func(fbr *fiber) (bool, exception) {
				a, exc := left(fbr)
				if exc != nil {
					return false, exc
				}
				return !a.Equals(b), nil
			}, true
		}
	case local:
		`)
	p(`	}
	return nil, false
}
`)

	src, err := format.Source(b.Bytes())
	if err != nil {
		os.Stdout.Write(b.Bytes())
		fmt.Fprintln(os.Stderr, "gen_ops: generated code does not parse:", err)
		os.Exit(1)
	}
	if err := os.WriteFile("ops_gen.go", src, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
