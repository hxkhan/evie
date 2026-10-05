package vm

import (
	"fmt"
)

type capture struct {
	isLocal bool // isLocal: capture from the parent's stack; !isLocal: capture from the parent's captures
	index   int
}

func (c capture) String() string {
	if c.isLocal {
		return fmt.Sprintf("Local(%v)", c.index)
	}
	return fmt.Sprintf("Captured(%v)", c.index)
}

// funcInfoStatic holds static function information
type funcInfoStatic struct {
	name       string      // name of the function
	args       []string    // argument names, including the receiver of a method
	isVariadic bool        // variadic function (...args)
	isMethod   bool        // declared inside a struct; args[0] is the receiver
	minArgs    int         // fewest arguments a call may pass, receiver included
	maxArgs    int         // most arguments a call may pass; unbounded when variadic
	locals     []bool      // all locals & true for those that escape
	captures   []capture   // captured references
	recyclable int         // number of non-escaping locals
	code       instruction // the actual function code
	def        site        // where the function was declared
	vm         *Instance   // the corresponding vm
}

// newFuncInfo describes a function before its body is compiled
func (vm *Instance) newFuncInfo(name string, args []string, isVariadic bool, isMethod bool) *funcInfoStatic {
	info := &funcInfoStatic{name: name, args: args, isVariadic: isVariadic, isMethod: isMethod, vm: vm}
	info.minArgs, info.maxArgs = len(args), len(args)
	if isVariadic {
		info.minArgs, info.maxArgs = len(args)-1, int(^uint(0)>>1)
	}
	return info
}

// accepts checks a call's argument count against the signature
func (fn *funcInfoStatic) accepts(n int) exception {
	if n >= fn.minArgs && n <= fn.maxArgs {
		return nil
	}
	return fn.arityError(n)
}

func (fn *funcInfoStatic) arityError(n int) exception {
	want, got := len(fn.args), n
	if fn.isMethod {
		want, got = want-1, got-1
	}
	if fn.isVariadic {
		return customError("%s takes at least %s but %s supplied", fn.describe(), plural(want-1, "argument"), supplied(got))
	}
	return customError("%s takes %s but %s supplied", fn.describe(), plural(want, "argument"), supplied(got))
}

// plural renders a count with its noun: "1 argument", "2 arguments", "no arguments"
func plural(n int, noun string) string {
	switch n {
	case 0:
		return "no " + noun + "s"
	case 1:
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// supplied renders how many arguments a call passed
func supplied(n int) string {
	if n == 1 {
		return "1 was"
	}
	return fmt.Sprintf("%d were", n)
}

func (fn *funcInfoStatic) describe() string {
	if fn.name == "" {
		return "callable"
	}
	if fn.isMethod {
		return fmt.Sprintf("method '%v'", fn.name)
	}
	return fmt.Sprintf("function '%v'", fn.name)
}

type userFn struct {
	*funcInfoStatic
	references []*Value // captured variables
}

// ---- the frame protocol ----
//
// Every call of an evie function is the same four steps: push the callee's
// locals, evaluate or copy the arguments into the first slots, run the body with
// base and active pointing at the new frame, then release the frame and
// translate the return signal into a plain result. Go does not inline a
// function that does all four, and a call costs about as much as the steps
// themselves, so the entry points below spell them out instead of sharing
// them: invoke, invokeMethod, call and the static call sites in emitCall.
// Only the variadic tail, which allocates anyway, is shared.

// evalVariadicTail evaluates the arguments from index fixed onwards into an
// array in the slot after the fixed parameters
func (fbr *fiber) evalVariadicTail(base int, slot int, fixed int, arguments []instruction) exception {
	tail := make([]Value, len(arguments)-fixed)
	for t := range tail {
		arg, exc := arguments[fixed+t](fbr)
		if exc != nil {
			return exc
		}
		tail[t] = arg
	}
	*(fbr.stack[base+slot]) = BoxArray(&Array{Data: tail})
	return nil
}

// storeVariadicTail packs already evaluated arguments from index fixed onwards
func (fbr *fiber) storeVariadicTail(base int, slot int, fixed int, args []Value) {
	tail := make([]Value, len(args)-fixed)
	copy(tail, args[fixed:])
	*(fbr.stack[base+slot]) = BoxArray(&Array{Data: tail})
}

// callGuarded invokes the function from Go. A nil fiber borrows one from the instance.
// The calling fiber becomes the instance's running fiber for the duration of the
// call unless it already is, which is the case for callbacks made from natives.
func (fn *userFn) callGuarded(fbr *fiber, args ...Value) (result Value, err exception) {
	if fbr == nil {
		if fn.vm == nil {
			return Value{}, customError("cannot call function without a VM instance")
		}
		fbr = fn.vm.newFiber()
		defer fn.vm.releaseFiber(fbr)
	}

	if exc := fn.accepts(len(args)); exc != nil {
		return Value{}, exc
	}

	if fbr.lock() {
		defer fbr.unlock()
	}

	stack, base, active := len(fbr.stack), fbr.base, fbr.active
	defer func() {
		if r := recover(); r != nil {
			result, err = Value{}, recovered(r)
			fbr.unwind(stack, base, active)
		}
	}()

	return fn.call(fbr, args)
}

// call runs the function on a fiber that is already running with arguments
// that are known to match the signature. Nothing here recovers from a panic;
// that is the job of the host boundary in Call.
func (fn *userFn) call(fbr *fiber, args []Value) (Value, exception) {
	base := len(fbr.stack)
	for _, escapes := range fn.locals {
		if !escapes {
			fbr.stack = append(fbr.stack, fbr.pop())
		} else {
			fbr.stack = append(fbr.stack, &Value{})
		}
	}

	if !fn.isVariadic {
		for idx, arg := range args {
			*(fbr.stack[base+idx]) = arg
		}
	} else {
		fixed := len(fn.args) - 1
		for idx := range fixed {
			*(fbr.stack[base+idx]) = args[idx]
		}
		fbr.storeVariadicTail(base, fixed, fixed, args)
	}

	prevBase, prevActive := fbr.base, fbr.active
	fbr.base, fbr.active = base, fn

	result, exc := fn.code(fbr)

	fbr.push(fn.recyclable)
	fbr.popStack(len(fn.locals))
	fbr.base, fbr.active = prevBase, prevActive

	switch exc {
	case nil:
		return Value{}, nil
	case signalReturn:
		return result, nil
	default:
		return result, exc
	}
}

// invoke evaluates the arguments straight into a new frame and runs the
// function; the argument count is known to match the signature
func (fn *userFn) invoke(fbr *fiber, site site, arguments []instruction) (Value, exception) {
	base := len(fbr.stack)
	for _, escapes := range fn.locals {
		if !escapes {
			fbr.stack = append(fbr.stack, fbr.pop())
		} else {
			fbr.stack = append(fbr.stack, &Value{})
		}
	}

	fixed := len(arguments)
	if fn.isVariadic {
		fixed = len(fn.args) - 1
	}
	for idx := range fixed {
		arg, exc := arguments[idx](fbr)
		if exc != nil {
			fbr.push(fn.recyclable)
			fbr.popStack(len(fn.locals))
			return Value{}, exc
		}
		*(fbr.stack[base+idx]) = arg
	}
	if fn.isVariadic {
		if exc := fbr.evalVariadicTail(base, fixed, fixed, arguments); exc != nil {
			fbr.push(fn.recyclable)
			fbr.popStack(len(fn.locals))
			return Value{}, exc
		}
	}

	prevBase, prevActive := fbr.base, fbr.active
	fbr.base, fbr.active = base, fn

	result, exc := fn.code(fbr)

	fbr.push(fn.recyclable)
	fbr.popStack(len(fn.locals))
	fbr.base, fbr.active = prevBase, prevActive

	switch exc {
	case nil:
		return Value{}, nil
	case signalReturn:
		return result, nil
	default:
		return result, exc.unwind(site)
	}
}

// invokeMethod is invoke with the receiver placed in slot 0
func (fn *userFn) invokeMethod(fbr *fiber, site site, this Value, arguments []instruction) (Value, exception) {
	base := len(fbr.stack)
	for _, escapes := range fn.locals {
		if !escapes {
			fbr.stack = append(fbr.stack, fbr.pop())
		} else {
			fbr.stack = append(fbr.stack, &Value{})
		}
	}
	*(fbr.stack[base]) = this

	fixed := len(arguments)
	if fn.isVariadic {
		fixed = len(fn.args) - 2
	}
	for idx := range fixed {
		arg, exc := arguments[idx](fbr)
		if exc != nil {
			fbr.push(fn.recyclable)
			fbr.popStack(len(fn.locals))
			return Value{}, exc
		}
		*(fbr.stack[base+1+idx]) = arg
	}
	if fn.isVariadic {
		if exc := fbr.evalVariadicTail(base, 1+fixed, fixed, arguments); exc != nil {
			fbr.push(fn.recyclable)
			fbr.popStack(len(fn.locals))
			return Value{}, exc
		}
	}

	prevBase, prevActive := fbr.base, fbr.active
	fbr.base, fbr.active = base, fn

	result, exc := fn.code(fbr)

	fbr.push(fn.recyclable)
	fbr.popStack(len(fn.locals))
	fbr.base, fbr.active = prevBase, prevActive

	switch exc {
	case nil:
		return Value{}, nil
	case signalReturn:
		return result, nil
	default:
		return result, exc.unwind(site)
	}
}

// boundMethod is a member extracted from a value without calling it, such as
// `fnVal := p.greet`; calling it supplies the receiver again
type boundMethod struct {
	this Value
	fn   Value
}

func (m boundMethod) call(fbr *fiber, site site, arguments []instruction) (Value, exception) {
	if ufn, isUserFn := m.fn.asUserFn(); isUserFn {
		if ufn.isMethod {
			if exc := ufn.accepts(1 + len(arguments)); exc != nil {
				return Value{}, exc.at(site)
			}
			return ufn.invokeMethod(fbr, site, m.this, arguments)
		}
		if exc := ufn.accepts(len(arguments)); exc != nil {
			return Value{}, exc.at(site)
		}
		return ufn.invoke(fbr, site, arguments)
	}

	if fn, isGoFunc := m.fn.AsGoFunc(); isGoFunc {
		if fn.isMethod {
			return fn.callMethod(fbr, site, m.this, arguments)
		}
		// a native stored in an object field is a plain function, not a method of the object
		return fn.call(fbr, site, arguments)
	}

	// whatever else a field holds is called as a plain value
	args := make([]Value, len(arguments))
	for i, arg := range arguments {
		v, exc := arg(fbr)
		if exc != nil {
			return v, exc
		}
		args[i] = v
	}
	result, exc := m.fn.callGuarded(fbr, args...)
	if exc != nil {
		exc = exc.at(site)
	}
	return result, exc
}

func (m boundMethod) callGuarded(fbr *fiber, args ...Value) (Value, exception) {
	withReceiver := func() []Value {
		all := make([]Value, 1+len(args))
		all[0] = m.this
		copy(all[1:], args)
		return all
	}

	if ufn, isUserFn := m.fn.asUserFn(); isUserFn {
		if ufn.isMethod {
			return ufn.callGuarded(fbr, withReceiver()...)
		}
		return ufn.callGuarded(fbr, args...)
	}

	if fn, isGoFunc := m.fn.AsGoFunc(); isGoFunc {
		if fn.isMethod {
			return fn.callGuarded(fbr, withReceiver()...)
		}
		return fn.callGuarded(fbr, args...)
	}

	return m.fn.callGuarded(fbr, args...)
}

// GoFunc is a native function exposed to evie code. Arguments counts the
// receiver of a method. A Blocking function gives up the instance runner while
// it executes, so it must not touch evie values that other fibers can reach;
// use it for I/O and sleeps.
type GoFunc struct {
	Name       string
	arguments  int
	isVariadic bool
	isMethod   bool
	Blocking   bool
	fn         NativeFn
}

func (fn *GoFunc) accepts(n int) exception {
	if fn.isVariadic {
		if n >= fn.arguments {
			return nil
		}
	} else if n == fn.arguments {
		return nil
	}
	return fn.arityError(n)
}

func (fn *GoFunc) arityError(n int) exception {
	want, got := fn.arguments, n
	if fn.isMethod {
		want, got = want-1, got-1
	}
	name := "this function"
	if fn.Name != "" {
		name = "'" + fn.Name + "'"
	}
	if fn.isVariadic {
		return customError("%s takes at least %s but %s supplied", name, plural(want, "argument"), supplied(got))
	}
	return customError("%s takes %s but %s supplied", name, plural(want, "argument"), supplied(got))
}

// run executes the native on the frame at base, where its arguments are, and
// releases the frame
func (fn *GoFunc) run(fbr *fiber, base int, pushed int) (Value, exception) {
	prevBase := fbr.swapBase(base)
	c := Call{fbr: fbr, fn: fn, base: base, n: pushed}

	var result Value
	var err error
	if fn.Blocking && fbr.running {
		fbr.unlock()
		result, err = fn.fn(c)
		fbr.lock()
	} else {
		result, err = fn.fn(c)
	}

	fbr.push(pushed)
	fbr.popStack(pushed)
	fbr.swapBase(prevBase)
	return result, toException(err)
}

// pushArg places one evaluated argument in a fresh box on the stack
func (fbr *fiber) pushArg(v Value) {
	box := fbr.pop()
	*box = v
	fbr.stack = append(fbr.stack, box)
}

// abandonArgs releases the n argument boxes pushed so far
func (fbr *fiber) abandonArgs(n int) {
	fbr.push(n)
	fbr.popStack(n)
}

func (fn *GoFunc) call(fbr *fiber, site site, arguments []instruction) (Value, exception) {
	if exc := fn.accepts(len(arguments)); exc != nil {
		return Value{}, exc.at(site)
	}
	return fn.invoke(fbr, site, arguments)
}

// invoke evaluates the arguments onto the stack and runs the native; the
// argument count is known to match. An error the native returns is located
// at the call.
func (fn *GoFunc) invoke(fbr *fiber, site site, arguments []instruction) (Value, exception) {
	base := len(fbr.stack)
	for i, arg := range arguments {
		v, exc := arg(fbr)
		if exc != nil {
			fbr.abandonArgs(i)
			return v, exc
		}
		fbr.pushArg(v)
	}
	result, exc := fn.run(fbr, base, len(arguments))
	if exc != nil {
		exc = exc.at(site)
	}
	return result, exc
}

// callMethod is invoke with the receiver as the first argument
func (fn *GoFunc) callMethod(fbr *fiber, site site, this Value, arguments []instruction) (Value, exception) {
	if exc := fn.accepts(1 + len(arguments)); exc != nil {
		return Value{}, exc.at(site)
	}

	base := len(fbr.stack)
	fbr.pushArg(this)
	for i, arg := range arguments {
		v, exc := arg(fbr)
		if exc != nil {
			fbr.abandonArgs(1 + i)
			return v, exc
		}
		fbr.pushArg(v)
	}
	result, exc := fn.run(fbr, base, 1+len(arguments))
	if exc != nil {
		exc = exc.at(site)
	}
	return result, exc
}

// callGuarded invokes the native from Go; a nil fiber is not allowed because a native
// needs a stack to read its arguments from
func (fn *GoFunc) callGuarded(fbr *fiber, args ...Value) (result Value, exc exception) {
	if fbr == nil {
		return Value{}, customError("cannot call GoFunc with nil fiber")
	}

	if exc := fn.accepts(len(args)); exc != nil {
		return Value{}, exc
	}

	if fbr.lock() {
		defer fbr.unlock()
	}

	base := len(fbr.stack)
	entryBase, entryActive := fbr.base, fbr.active
	defer func() {
		if r := recover(); r != nil {
			result, exc = Value{}, recovered(r)
			fbr.unwind(base, entryBase, entryActive)
		}
	}()

	for _, arg := range args {
		fbr.pushArg(arg)
	}
	return fn.run(fbr, base, len(args))
}
