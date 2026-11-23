package vm

import (
	"fmt"
	goruntime "runtime"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/internal/fields"
)

type fieldsID = fields.ID

// Error is an exception: what went wrong, where, and the calls that led
// there. It is what `catch` turns into an error value and what the host gets
// back from a call that failed.
type Error struct {
	diag.Report
	Cause error

	code  int  // distinguishes internal control-flow signals
	fatal bool // the run was stopped; catch lets it pass
}

type exception = *Error

var signalReturn = &Error{Report: diag.Report{Message: "return"}, code: 0}
var signalContinue = &Error{Report: diag.Report{Message: "continue"}, code: 1}
var signalBreak = &Error{Report: diag.Report{Message: "break"}, code: 2}

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
	pos diag.Pos
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

func customError(msg string, a ...any) exception {
	return &Error{Report: diag.Report{Message: fmt.Sprintf(msg, a...)}}
}

func runtimeErrorf(format string, a ...any) exception {
	return customError(format, a...)
}

func argTypeErrorf(format string, a ...any) exception {
	return customError(format, a...)
}

// errorAt raises an error at a site
func errorAt(s site, format string, a ...any) exception {
	return customError(format, a...).at(s)
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

// clone copies an error for a new owner, trace included, so that locating or
// unwinding the copy leaves the original alone. fatal says whether the copy
// still stops whoever gets it.
func (e *Error) clone(fatal bool) *Error {
	c := *e
	c.Trace = slices.Clone(e.Trace)
	c.fatal = fatal
	return &c
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
func memberError(s site, name string, v Value) exception {
	e := errorAt(s, "no member '%s' on %s", name, article(v))
	if closest, ok := diag.Suggest(name, v.memberNames()); ok {
		e.Hint = fmt.Sprintf("%s has a member with a similar name: '%s'", article(v), closest)
	}
	if pkg, ok := v.AsPackage(); ok && pkg.memberHint != "" {
		e.Hint = pkg.memberHint
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
	if ci, ok := v.asClassInstance(); ok {
		t = ci.InstanceOf.Name
	}
	if t == "nil" || len(t) == 0 {
		return t
	}
	if strings.ContainsRune("aeiouAEIOU", rune(t[0])) {
		return "an " + t
	}
	return "a " + t
}

// operatorError explains a binary operator applied to the wrong operands
func operatorError(s site, op string, a Value, b Value) exception {
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

func argTypeError(args []Value, expected ...string) exception {
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

	return customError("%s", msg)
}

// recovered turns a panic that reached a host boundary into an exception. A
// thrown exception or compile error passes through; anything else is a bug in
// the runtime or in a native and is reported with its stack instead of taking
// the host process down.
// loadError turns what compiling or loading code panicked with into the error
// the host gets: compile errors, exceptions and other errors (such as an
// import's own report) as they are, and a Go runtime panic, which is a bug, as
// an internal error with its stack
func loadError(r any) error {
	switch e := r.(type) {
	case *CompileError:
		return e
	case exception:
		return e
	case goruntime.Error:
		return recovered(r)
	case error:
		return e
	}
	return recovered(r)
}

func recovered(r any) exception {
	switch e := r.(type) {
	case exception:
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

func compileErrorf(format string, a ...any) *CompileError {
	return &CompileError{Report: diag.Report{Message: fmt.Sprintf(format, a...)}}
}

// compileErrorAt reports a compile error at a node of the source being compiled
func (vm *Instance) compileErrorAt(node ast.Node, format string, a ...any) *CompileError {
	e := compileErrorf(format, a...)
	e.Source, e.Pos = vm.cp.src, node.Where()
	return e
}

// compileErrorAtPos reports a compile error at a position in the source being compiled
func (vm *Instance) compileErrorAtPos(pos diag.Pos, format string, a ...any) *CompileError {
	e := compileErrorf(format, a...)
	e.Source, e.Pos = vm.cp.src, pos
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
	if pkg, ok := v.AsPackage(); ok && pkg.memberHint != "" {
		ce.Hint = pkg.memberHint
	}
	return ce
}

// checkPackageMember stops compiling when fa names a member that an imported
// package does not have. A package is a constant, so this is known while
// compiling, whether the member is read, called or assigned and whether or not
// the fast paths are on.
func (vm *Instance) checkPackageMember(fa ast.FieldAccess) {
	ident, ok := fa.Lhs.(ast.Ident)
	if !ok {
		return
	}
	pkg := vm.cp.packageNamed(ident.Name)
	if pkg == nil {
		return
	}
	if id, known := fields.Find(fa.Rhs); known {
		if g, exists := pkg.globals[id]; exists && g.IsPublic {
			return
		}
	}
	panic(vm.memberCompileError(fa, fa.Rhs, pkg.Box()))
}

// locate gives a compile error that was raised without a position the node it concerns
func (vm *Instance) locate(err error, node ast.Node) error {
	if ce, ok := err.(*CompileError); ok && !ce.Pos.Known() {
		ce.Source, ce.Pos = vm.cp.src, node.Where()
	}
	return err
}

// CompileErrors is what compiling a package returns when it finds more than
// one compile error, in the order it found them. errors.As finds each
// *CompileError in it.
type CompileErrors []*CompileError

func (l CompileErrors) Error() string {
	if len(l) == 1 {
		return l[0].Error()
	}
	return fmt.Sprintf("%s (and %d more)", l[0].Error(), len(l)-1)
}

func (l CompileErrors) Unwrap() []error {
	errs := make([]error, len(l))
	for i, e := range l {
		errs[i] = e
	}
	return errs
}

// Diagnostics returns the report of every error, for tools that show them all
func (l CompileErrors) Diagnostics() []*diag.Report {
	reports := make([]*diag.Report, len(l))
	for i, e := range l {
		reports[i] = &e.Report
	}
	return reports
}

// maxCompileErrors is how many compile errors one package reports before
// compiling it stops
const maxCompileErrors = 10

// tolerate runs compile, a part of a package that compiles on its own such as
// a function body. A compile error in it is recorded rather than ending the
// package, so one mistake does not hide the others; the compiler's state is
// put back as it was before. Anything else compile stops with passes on, and
// outside a package being compiled every error does. It reports whether
// compile finished.
func (vm *Instance) tolerate(compile func()) (ok bool) {
	cp := &vm.cp
	if cp.errs == nil {
		compile()
		return true
	}
	closures, src := cp.closures.Len(), cp.src
	var cl *closure
	var depth, loops, switches int
	if closures > 0 {
		cl = cp.closures.Last(0)
		depth, loops, switches = cl.scope.Depth(), cl.loops, cl.switches
	}
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		ce, isCompileError := r.(*CompileError)
		if !isCompileError {
			panic(r)
		}
		for cp.closures.Len() > closures {
			cp.closures.Pop()
		}
		if cl != nil {
			cl.scope.CloseTo(depth)
			cl.loops, cl.switches = loops, switches
		}
		cp.src = src
		*cp.errs = append(*cp.errs, ce)
		if len(*cp.errs) >= maxCompileErrors {
			panic(cp.failed())
		}
		ok = false
	}()
	compile()
	return true
}

// failed is the error the compile errors recorded so far make, or nil
func (cp *compiler) failed() error {
	if cp.errs == nil {
		return nil
	}
	switch errs := *cp.errs; len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		return CompileErrors(slices.Clone(errs))
	}
}
