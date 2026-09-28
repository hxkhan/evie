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
	locals     []bool      // all locals & true for those that escape
	captures   []capture   // captured references
	recyclable int         // number of non-escaping locals
	code       instruction // the actual function code
	vm         *Instance   // the corresponding vm
}

// accepts checks a call's argument count against the signature
func (fn *funcInfoStatic) accepts(n int) Exception {
	if fn.isVariadic {
		if minArgs := len(fn.args) - 1; n < minArgs {
			return CustomError("%s requires at least %v argument(s), %v provided", fn.describe(), minArgs, n)
		}
		return nil
	}
	if n != len(fn.args) {
		return CustomError("%s requires %v argument(s), %v provided", fn.describe(), len(fn.args), n)
	}
	return nil
}

func (fn *funcInfoStatic) describe() string {
	if fn.name == "" {
		return "callable"
	}
	return fmt.Sprintf("function '%v'", fn.name)
}

type UserFn struct {
	*funcInfoStatic
	references []*Value // captured variables
}

// Call invokes the function from Go. A nil fiber borrows one from the instance.
// The calling fiber becomes the instance's running fiber for the duration of the
// call unless it already is, which is the case for callbacks made from natives.
func (fn *UserFn) Call(fbr *Fiber, args ...Value) (result Value, err Exception) {
	if fbr == nil {
		if fn.vm == nil {
			return Value{}, CustomError("cannot call function without a VM instance")
		}
		fbr = fn.vm.NewFiber()
		defer fn.vm.ReleaseFiber(fbr)
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

// call runs the function on a fiber that is already running. The arguments are
// known to match the signature and nothing here recovers from a panic; that is
// the job of the host boundary in Call.
func (fn *UserFn) call(fbr *Fiber, args []Value) (result Value, err Exception) {
	base := len(fbr.stack)

	// create space for all the locals
	for idx, escapes := range fn.locals {
		if !escapes {
			fbr.stack = append(fbr.stack, fbr.pop())
		} else {
			fbr.stack = append(fbr.stack, &Value{})
		}

		// assign arguments
		if !fn.isVariadic {
			if idx < len(args) {
				*fbr.stack[base+idx] = args[idx]
			}
		} else {
			variadicIdx := len(fn.args) - 1
			if idx < variadicIdx {
				*fbr.stack[base+idx] = args[idx]
			} else if idx == variadicIdx {
				tailCount := len(args) - variadicIdx
				tail := make([]Value, tailCount)
				copy(tail, args[variadicIdx:])
				*fbr.stack[base+idx] = BoxArray(&Array{Data: tail})
			}
		}
	}

	// save current state
	prevBase := fbr.swapBase(base)
	prevActive := fbr.swapActive(fn)

	// call the function
	result, exc := fn.code(fbr)

	// restore old state
	fbr.push(fn.recyclable)
	fbr.popStack(len(fn.locals))
	fbr.swapBase(prevBase)
	fbr.swapActive(prevActive)

	// don't implicitly return the return value of the last executed instruction
	switch exc {
	case nil:
		return Value{}, nil
	case signalReturn:
		return result, nil
	default:
		return result, exc
	}
}

type BoundMethod struct {
	this Value
	fn   Value
}

func (m BoundMethod) call(fbr *Fiber, arguments []instruction) (result Value, exc Exception) {
	if ufn, isUserFn := m.fn.AsUserFn(); isUserFn {
		if ufn.isMethod {
			if exc := ufn.accepts(1 + len(arguments)); exc != nil {
				return Value{}, exc
			}

			args := make([]Value, 1+len(arguments))
			args[0] = m.this
			for i, arg := range arguments {
				v, exc := arg(fbr)
				if exc != nil {
					return v, exc
				}
				args[1+i] = v
			}

			return ufn.call(fbr, args)
		}

		if exc := ufn.accepts(len(arguments)); exc != nil {
			return Value{}, exc
		}

		args := make([]Value, len(arguments))
		for i, arg := range arguments {
			v, exc := arg(fbr)
			if exc != nil {
				return v, exc
			}
			args[i] = v
		}

		return ufn.call(fbr, args)
	}

	fn, ok := m.fn.AsGoFunc()
	if !ok {
		// whatever else a field holds is called as a plain value
		args := make([]Value, len(arguments))
		for i, arg := range arguments {
			v, exc := arg(fbr)
			if exc != nil {
				return v, exc
			}
			args[i] = v
		}
		return m.fn.Call(fbr, args...)
	}

	// a native stored in an object field is a plain function, not a method of the object
	if !fn.IsMethod {
		return fn.call(fbr, arguments)
	}

	if exc := fn.accepts(1 + len(arguments)); exc != nil {
		return Value{}, exc
	}

	base := len(fbr.stack)

	// push self
	box := fbr.pop()
	*box = m.this
	fbr.stack = append(fbr.stack, box)

	for i, arg := range arguments {
		v, exc := arg(fbr)
		if exc != nil {
			// release self and the arguments evaluated so far
			fbr.push(1 + i)
			fbr.popStack(1 + i)
			return v, exc
		}

		box := fbr.pop()
		*box = v
		fbr.stack = append(fbr.stack, box)
	}

	prevBase := fbr.swapBase(base)
	result, exc = fn.run(fbr)

	totalPushed := 1 + len(arguments)
	fbr.push(totalPushed)
	fbr.popStack(totalPushed)
	fbr.swapBase(prevBase)

	return result, exc
}

func (m BoundMethod) Call(fbr *Fiber, args ...Value) (result Value, exc Exception) {
	if ufn, isUserFn := m.fn.AsUserFn(); isUserFn {
		if ufn.isMethod {
			allArgs := make([]Value, 1+len(args))
			allArgs[0] = m.this
			copy(allArgs[1:], args)
			return ufn.Call(fbr, allArgs...)
		}
		return ufn.Call(fbr, args...)
	}

	fn, ok := m.fn.AsGoFunc()
	if !ok {
		return m.fn.Call(fbr, args...)
	}

	if !fn.IsMethod {
		return fn.Call(fbr, args...)
	}

	allArgs := make([]Value, 1+len(args))
	allArgs[0] = m.this
	copy(allArgs[1:], args)
	return fn.Call(fbr, allArgs...)
}

type GoFuncSignature = func(fbr *Fiber) (Value, Exception)

// GoFunc is a native function exposed to evie code. A Blocking function gives up
// the instance runner while it executes, so it must not touch evie values that
// other fibers can reach; use it for I/O and sleeps.
type GoFunc struct {
	Name       string
	Arguments  int
	IsVariadic bool
	IsMethod   bool
	Blocking   bool
	Fn         GoFuncSignature
}

func (fn *GoFunc) accepts(n int) Exception {
	if fn.IsVariadic {
		if n < fn.Arguments {
			return CustomError("callable requires at least %v argument(s), %v provided", fn.Arguments, n)
		}
		return nil
	}
	if fn.Arguments != n {
		return CustomError("callable requires %v argument(s), %v provided", fn.Arguments, n)
	}
	return nil
}

// run executes the native with its arguments already on the fiber stack
func (fn *GoFunc) run(fbr *Fiber) (Value, Exception) {
	if fn.Blocking && fbr.running {
		fbr.unlock()
		defer fbr.lock()
	}
	return fn.Fn(fbr)
}

func (fn *GoFunc) call(fbr *Fiber, arguments []instruction) (result Value, exc Exception) {
	if exc := fn.accepts(len(arguments)); exc != nil {
		return Value{}, exc
	}
	return fn.invoke(fbr, arguments)
}

// just call; no args check
func (fn *GoFunc) invoke(fbr *Fiber, arguments []instruction) (result Value, exc Exception) {
	base := len(fbr.stack)
	for i, arg := range arguments {
		v, exc := arg(fbr)
		if exc != nil {
			// release the arguments evaluated so far
			fbr.push(i)
			fbr.popStack(i)
			return v, exc
		}

		box := fbr.pop()
		*box = v
		fbr.stack = append(fbr.stack, box)
	}

	prevBase := fbr.swapBase(base)
	result, exc = fn.run(fbr)

	fbr.push(len(arguments))
	fbr.popStack(len(arguments))
	fbr.swapBase(prevBase)

	return result, exc
}

// Call invokes the native from Go; a nil fiber is not allowed because a native
// needs a stack to read its arguments from
func (fn *GoFunc) Call(fbr *Fiber, args ...Value) (result Value, exc Exception) {
	if fbr == nil {
		return Value{}, CustomError("cannot call GoFunc with nil fiber")
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
		box := fbr.pop()
		*box = arg
		fbr.stack = append(fbr.stack, box)
	}

	prevBase := fbr.swapBase(base)
	result, exc = fn.run(fbr)

	fbr.push(len(args))
	fbr.popStack(len(args))
	fbr.swapBase(prevBase)

	return result, exc
}
