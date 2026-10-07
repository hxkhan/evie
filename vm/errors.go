package vm

import (
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/token"
	"github.com/hxkhan/evie/vm/fields"
)

type fieldsID = fields.ID

// Error is an exception: what went wrong, where, and the calls that led
// there. It is what `catch` turns into an error value and what the host gets
// back from a call that failed.
type Error struct {
	diag.Report
	Code  int
	Cause error
}

type Exception = *Error

var signalReturn = &Error{Report: diag.Report{Message: "return"}, Code: 0}
var signalContinue = &Error{Report: diag.Report{Message: "continue"}, Code: 1}
var signalBreak = &Error{Report: diag.Report{Message: "break"}, Code: 2}

func (e *Error) Error() string {
	return e.Report.Error()
}

func (e *Error) Unwrap() error {
	return e.Cause
}

// isSignal reports whether e steers control flow rather than reporting a failure
func (e *Error) isSignal() bool {
	return e == signalReturn || e == signalContinue || e == signalBreak
}

// site is where an instruction came from: the source, the span and the
// function it belongs to. Instructions capture one when they are compiled so
// that an error raised at run time can say where it happened at no cost on
// the path where nothing fails.
type site struct {
	src *diag.Source
	pos token.Pos
	fn  string
}

// unknownSite is the site of code that has no source position
var unknownSite site

func (s site) known() bool {
	return s.pos.Known()
}

// at records the instruction being compiled as a site
func (vm *Instance) at(node ast.Node) site {
	fn := ""
	if vm.cp.closures.Len() > 0 {
		fn = vm.cp.closures.Last(0).info.name
	}
	return site{src: vm.cp.src, pos: node.Where(), fn: fn}
}

func CustomError(msg string, a ...any) Exception {
	return &Error{Report: diag.Report{Message: fmt.Sprintf(msg, a...)}}
}

func RuntimeExceptionF(format string, a ...any) Exception {
	return CustomError(format, a...)
}

func TypeErrorF(format string, a ...any) Exception {
	return CustomError(format, a...)
}

// errorAt raises an error at a site
func errorAt(s site, format string, a ...any) Exception {
	return CustomError(format, a...).at(s)
}

// at locates an error that has no location yet and opens its trace there
func (e *Error) at(s site) *Error {
	if e.isSignal() || e.Pos.Known() || !s.known() {
		return e
	}
	e.Source, e.Pos = s.src, s.pos
	if e.Trace == nil {
		e.Trace = make([]diag.Frame, 0, 4) // most traces are short; one allocation covers them
	}
	e.Trace = append(e.Trace, diag.Frame{Function: s.fn, Source: s.src, Pos: s.pos})
	return e
}

// unwind records that the error passed through a call made at s
func (e *Error) unwind(s site) *Error {
	if e.isSignal() || !s.known() {
		return e
	}
	e.Trace = append(e.Trace, diag.Frame{Function: s.fn, Source: s.src, Pos: s.pos})
	return e
}

// field exposes the error to scripts: message, line, column, file and trace
func (e *Error) field(f fieldsID) (Value, bool) {
	switch f {
	case messageID:
		return BoxString(e.Message), true
	case lineID:
		return BoxNumber(float64(e.Pos.Line)), true
	case columnID:
		return BoxNumber(float64(e.Pos.Column)), true
	case fileID:
		if e.Source != nil {
			return BoxString(e.Source.Name), true
		}
		return Value{}, true
	case traceID:
		frames := make([]Value, len(e.Trace))
		for i, fr := range e.Trace {
			frames[i] = BoxString(fmt.Sprintf("%s at %s", fr.Function, (&diag.Report{Source: fr.Source, Pos: fr.Pos}).Location()))
		}
		return BoxArray(NewArray(frames...)), true
	}
	return Value{}, false
}

// memberError explains a missing member and suggests a close one
func memberError(s site, name string, v Value) Exception {
	e := errorAt(s, "no member '%s' on %s", name, article(v))
	if closest, ok := diag.Suggest(name, v.memberNames()); ok {
		e.Hint = fmt.Sprintf("%s has a member with a similar name: '%s'", article(v), closest)
	}
	return e
}

func (e *Error) labelled(label string) *Error {
	e.Label = label
	return e
}

func (e *Error) hinted(hint string) *Error {
	e.Hint = hint
	return e
}

// article names a value's type the way a sentence needs it: "a number", "an
// array", "a Point", "nil"
func article(v Value) string {
	t := v.TypeOf()
	if usi, ok := v.AsUserStructInstance(); ok {
		t = usi.InstanceOf.Name
	}
	if t == "nil" {
		return t
	}
	if strings.ContainsRune("aeiouAEIOU", rune(t[0])) {
		return "an " + t
	}
	return "a " + t
}

// operatorError explains a binary operator applied to the wrong operands
func operatorError(s site, op string, a Value, b Value) Exception {
	e := errorAt(s, "cannot apply '%s' to %s and %s", op, article(a), article(b))
	e.Label = fmt.Sprintf("%s %s %s", a.TypeOf(), op, b.TypeOf())
	_, aStr := a.AsString()
	_, bStr := b.AsString()
	switch {
	case op == "+" && (aStr || bStr):
		e.Hint = "convert the other side with string(...) to concatenate, or with number(...) to add"
	case a.IsNil() || b.IsNil():
		e.Hint = "one side is nil; check that it was assigned before this point"
	}
	return e
}

func TypeError(args []Value, expected ...string) Exception {
	msg := "expected types ("
	for i, ex := range expected {
		msg += fmt.Sprintf("'%s'", ex)
		if i != len(expected)-1 {
			msg += ", "
		}
	}

	msg += "), got ("
	for i, arg := range args {
		msg += fmt.Sprintf("'%s'", arg.TypeOf())
		if i != len(args)-1 {
			msg += ", "
		}
	}

	return CustomError("%s", msg)
}

// recovered turns a panic that reached a host boundary into an exception. A
// thrown exception or compile error passes through; anything else is a bug in
// the runtime or in a native and is reported with its stack instead of taking
// the host process down.
func recovered(r any) Exception {
	switch e := r.(type) {
	case Exception:
		return e
	case *CompileError:
		return &Error{Report: e.Report}
	}
	return &Error{Report: diag.Report{Message: fmt.Sprintf("internal error: %v\n%s", r, debug.Stack())}}
}

// CompileError represents an error that occurred during compilation or static analysis.
type CompileError struct {
	diag.Report
}

func (e *CompileError) Error() string {
	return "compile error: " + e.Report.Error()
}

func CompileErrorf(format string, a ...any) *CompileError {
	return &CompileError{Report: diag.Report{Message: fmt.Sprintf(format, a...)}}
}

// compileErrorAt reports a compile error at a node of the source being compiled
func (vm *Instance) compileErrorAt(node ast.Node, format string, a ...any) *CompileError {
	e := CompileErrorf(format, a...)
	e.Source, e.Pos = vm.cp.src, node.Where()
	return e
}

func (e *CompileError) withHint(hint string) *CompileError {
	e.Hint = hint
	return e
}

// memberCompileError is memberError for a member resolved while compiling
func (vm *Instance) memberCompileError(node ast.Node, name string, v Value) *CompileError {
	ce := vm.compileErrorAt(node, "no member '%s' on %s", name, article(v))
	if closest, ok := diag.Suggest(name, v.memberNames()); ok {
		ce.Hint = fmt.Sprintf("%s has a member with a similar name: '%s'", article(v), closest)
	}
	return ce
}

// locate gives a compile error that was raised without a position the node it concerns
func (vm *Instance) locate(err error, node ast.Node) error {
	if ce, ok := err.(*CompileError); ok && !ce.Pos.Known() {
		ce.Source, ce.Pos = vm.cp.src, node.Where()
	}
	return err
}
