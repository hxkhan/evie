package vm

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
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
//
// Besides its message an error has a kind, such as "notFound" or "timeout",
// that says what sort of failure it is, so code can tell failures apart
// without reading messages; any fields a script gave it, as in
// error("no config", {path: p}); and the error it was raised because of, its
// cause. Cause is that error for the host, so errors.Is and errors.As see
// through it.
type Error struct {
	diag.Report
	Cause error

	kind   string
	cause  Value   // the cause as scripts see it; nil when there is none
	fields *Object // what the script gave the error besides its message, kind and cause

	code  int  // distinguishes internal control-flow signals
	fatal bool // the run was stopped; catch lets it pass
}

// The kinds of error the runtime and the standard library raise. A script
// can give an error any kind; these are the ones Evie itself uses.
const (
	KindError         = "error"         // the default, for errors without a more specific kind
	KindType          = "type"          // a value of the wrong type, a call with the wrong arguments, a constant assigned
	KindValue         = "value"         // the right type but a value that cannot be used, as in number("two")
	KindRange         = "range"         // an index outside an array, string or buffer
	KindMember        = "member"        // a field, key, method or package member that does not exist
	KindNotFound      = "notFound"      // a file or directory that does not exist
	KindExists        = "exists"        // a file or directory that already exists
	KindPermission    = "permission"    // access that is not allowed
	KindIO            = "io"            // any other failure to read or write
	KindTimeout       = "timeout"       // a wait that ran out of time
	KindCancelled     = "cancelled"     // a task that was cancelled
	KindSyntax        = "syntax"        // text that does not parse, as for json.decode
	KindStackOverflow = "stackOverflow" // too many calls running at once, usually recursion that never stops; see Options.MaxCallDepth
)

type exception = *Error

var signalReturn = &Error{Report: diag.Report{Message: "return"}, code: 0}
var signalContinue = &Error{Report: diag.Report{Message: "continue"}, code: 1}
var signalBreak = &Error{Report: diag.Report{Message: "break"}, code: 2}

// Errorf makes an error of a kind, for a native function to return: scripts
// see the kind as e.kind. Give it fields with With.
func Errorf(kind, format string, a ...any) *Error {
	return &Error{Report: diag.Report{Message: fmt.Sprintf(format, a...)}, kind: kind}
}

// Error returns the error's report as text
func (e *Error) Error() string {
	return e.Report.Error()
}

// Unwrap returns the Go error this one was made from, if any
func (e *Error) Unwrap() error {
	return e.Cause
}

// Diagnostic is the error's report, which names its kind unless that is the
// default, so a rendered error reads error[notFound]: ...
func (e *Error) Diagnostic() *diag.Report {
	if k := e.Kind(); k != KindError {
		e.Report.Code = k
	}
	return &e.Report
}

// Kind says what sort of failure the error is, such as "notFound"; see the
// Kind constants. An error raised by Go code without one takes it from the Go
// error it came from, as "notFound" from fs.ErrNotExist, and is "error"
// otherwise.
func (e *Error) Kind() string {
	if e.kind != "" {
		return e.kind
	}
	return kindOf(e.Cause)
}

// kindOf finds the kind of a Go error from the standard errors it wraps
func kindOf(err error) string {
	var exc *Error
	switch {
	case err == nil:
		return KindError
	case errors.Is(err, ErrCanceled), errors.Is(err, context.Canceled):
		return KindCancelled
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, os.ErrDeadlineExceeded):
		return KindTimeout
	case errors.Is(err, fs.ErrNotExist):
		return KindNotFound
	case errors.Is(err, fs.ErrExist):
		return KindExists
	case errors.Is(err, fs.ErrPermission):
		return KindPermission
	case errors.As(err, &exc):
		return exc.Kind()
	}
	return KindError
}

// With gives the error a field that scripts read as e.name, and returns it.
// The names message, kind, cause, file, line, column and trace are the
// error's own and cannot be fields.
func (e *Error) With(name string, v Value) *Error {
	if isOwnField(name) {
		panic(fmt.Sprintf("vm: '%s' is a field every error has; it cannot be set with With", name))
	}
	if e.fields == nil {
		e.fields = NewObject(2)
	}
	e.fields.Set(name, v)
	return e
}

// Field is a field the error was given, as with With or error(message, fields)
func (e *Error) Field(name string) (Value, bool) {
	if e.fields == nil {
		return Value{}, false
	}
	return e.fields.Get(name)
}

// FieldNames lists the fields the error was given, in the order they were given
func (e *Error) FieldNames() []string {
	if e.fields == nil {
		return nil
	}
	return e.fields.Keys()
}

// CauseValue is the cause as scripts see it: the error, or other value, the
// error was raised because of; nil when there is none
func (e *Error) CauseValue() Value {
	if e.cause.IsNil() && e.Cause != nil {
		if exc, ok := e.Cause.(*Error); ok {
			return boxError(exc)
		}
	}
	return e.cause
}

// setCause makes v the cause; an error also becomes the cause the host sees
func (e *Error) setCause(v Value) {
	e.cause = v
	if exc, ok := v.AsError(); ok {
		e.Cause = exc
	}
}

// take gives the error the fields of an object: kind and cause as such, and
// any other name as a field. message sets the message where allowed, as for
// a thrown object; error(message, fields) takes its message first instead.
func (e *Error) take(obj *Object, message bool) *Error {
	for name, v := range obj.All() {
		switch name {
		case "kind":
			kind, ok := v.AsString()
			if !ok {
				return Errorf(KindType, "an error's kind must be a string, got %s", article(v))
			}
			e.kind = kind
		case "cause":
			e.setCause(v)
		case "message":
			if !message {
				return Errorf(KindType, "the message is error's first argument, not one of its fields")
			}
			if s, ok := v.AsString(); ok {
				e.Message = s
			} else {
				e.Message = v.String()
			}
		case "file", "line", "column", "trace":
			return Errorf(KindType, "an error's '%s' is where it was thrown and cannot be given; use another name", name)
		default:
			if e.fields == nil {
				e.fields = NewObject(obj.Len())
			}
			e.fields.Set(name, v)
		}
	}
	return nil
}

// thrown makes an error of what a script throws that is not one: a string is
// the message, an object gives its fields as error(message, fields) does,
// and anything else is kept as the field value
func thrown(v Value) *Error {
	if s, ok := v.AsString(); ok {
		return customError("%s", s)
	}
	if obj, ok := v.AsObject(); ok {
		e := customError("%s", v.String())
		if err := e.take(obj, true); err != nil {
			return err
		}
		return e
	}
	return customError("%s", v.String()).With("value", v)
}

func isOwnField(name string) bool {
	switch name {
	case "message", "kind", "cause", "file", "line", "column", "trace":
		return true
	}
	return false
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

// customError makes an error of the default kind; unlike one converted from
// Go, it does not take a kind from its cause
func customError(msg string, a ...any) exception {
	return &Error{Report: diag.Report{Message: fmt.Sprintf(msg, a...)}, kind: KindError}
}

// errorAt raises an error of a kind at a site
func errorAt(s site, kind, format string, a ...any) exception {
	return Errorf(kind, format, a...).at(s)
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

// field exposes the error to scripts: message, kind, cause, line, column, file
// and trace, and the fields the script gave it
func (e *Error) field(f fieldsID) (Value, bool) {
	switch f {
	case messageID:
		return BoxString(e.Message), true
	case kindID:
		return BoxString(e.Kind()), true
	case causeID:
		return e.CauseValue(), true
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
	if e.fields != nil {
		return e.fields.Get(fields.Lookup(f))
	}
	return Value{}, false
}

// memberError explains a missing member and suggests a close one
func memberError(s site, name string, v Value) exception {
	e := errorAt(s, KindMember, "no member '%s' on %s", name, article(v))
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
	e := errorAt(s, KindType, "cannot apply '%s' to %s and %s", op, article(a), article(b))
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

// Error returns the report prefixed with "compile error: "
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

// Error returns the first error, and how many more there are
func (l CompileErrors) Error() string {
	if len(l) == 1 {
		return l[0].Error()
	}
	return fmt.Sprintf("%s (and %d more)", l[0].Error(), len(l)-1)
}

// Unwrap returns every error in the list, for errors.Is and errors.As
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
