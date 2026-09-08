package vm

import (
	"fmt"
	"reflect"

	"github.com/hxkhan/evie/ast"
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
	name       string       // name of the function
	args       []string     // argument names
	isVariadic bool         // variadic function (...args)
	locals     []bool       // all locals & true for those that escape
	captures   []capture    // captured references
	recyclable int          // number of non-escaping locals
	code       instruction  // the actual function code
	mode       ast.SyncMode // the sync mode of the action
	vm         *Instance    // the corresponding vm
}

type UserFn struct {
	*funcInfoStatic
	references []*Value // captured variables
}

func (fn UserFn) Synced() bool {
	return fn.mode == ast.SyncedMode
}

/* func (fn UserFn) String() string {
	return fmt.Sprintf("<function %v>", fn.name)
} */

func (fn *UserFn) Call(fbr *Fiber, args ...Value) (result Value, err Exception) {
	if fbr == nil {
		if fn.vm == nil {
			return Value{}, CustomError("cannot call function without a VM instance")
		}
		fbr = fn.vm.NewFiber()
	}

	if fn.isVariadic {
		minArgs := len(fn.args) - 1
		if len(args) < minArgs {
			if fn.name != "λ" {
				return Value{}, CustomError("function '%v' requires at least %v argument(s), %v provided", fn.name, minArgs, len(args))
			}
			return Value{}, CustomError("callable requires at least %v argument(s), %v provided", minArgs, len(args))
		}
	} else {
		if len(fn.args) != len(args) {
			if fn.name != "λ" {
				return Value{}, CustomError("function '%v' requires %v argument(s), %v provided", fn.name, len(fn.args), len(args))
			}
			return Value{}, CustomError("callable requires %v argument(s), %v provided", len(fn.args), len(args))
		}
	}

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
	var exc Exception
	if fn.mode == ast.UndefinedMode || fbr.synced {
		result, exc = fn.code(fbr)
	} else {
		fbr.vm.rt.AcquireGIL()
		fbr.synced = true
		result, exc = fn.code(fbr)
		fbr.synced = false
		fbr.vm.rt.ReleaseGIL()
	}

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

func (fn *UserFn) SaveInto(ptr any) (err error) {
	if fn.isVariadic {
		return CustomError("cannot reflect variadic function")
	}

	fun := reflect.ValueOf(ptr).Elem()

	if len(fn.args) != fun.Type().NumIn() {
		if fn.name != "λ" {
			return CustomError("function '%v' requires %v argument(s), %v provided", fn.name, len(fn.args), fun.Type().NumIn())
		}
		return CustomError("callable requires %v argument(s), %v provided", len(fn.args), fun.Type().NumIn())
	}

	resultKind := fun.Type().Out(0).Kind()

	wrapper := reflect.MakeFunc(fun.Type(), func(in []reflect.Value) (out []reflect.Value) {
		vm := fn.vm
		vm.rt.AcquireGIL()
		defer vm.rt.ReleaseGIL()

		// fetch a fiber and prepare it
		fbr := vm.rt.fibers.Get().(*Fiber)
		fbr.active = fn
		fbr.base = 0
		fbr.stack = fbr.stack[:0]

		// create space for all the locals
		for idx, escapes := range fn.locals {
			if !escapes {
				fbr.stack = append(fbr.stack, fbr.pop())
			} else {
				fbr.stack = append(fbr.stack, &Value{})
			}

			// assign arguments
			if idx < len(in) {
				v := in[idx]
				switch v.Kind() {
				case reflect.Int, reflect.Int32, reflect.Int64:
					*fbr.stack[idx] = BoxNumber(float64(v.Int()))
				case reflect.Float32, reflect.Float64:
					*fbr.stack[idx] = BoxNumber(v.Float())
				case reflect.String:
					*fbr.stack[idx] = BoxString(v.String())
				default:
					panic("Call: Unsuported types supplied!")
				}
			}
		}

		// prep for execution & save currently captured values
		result, err := fn.code(fbr)

		// release non-escaping locals and fiber
		fbr.push(fn.recyclable)
		vm.rt.fibers.Put(fbr)

		out = make([]reflect.Value, 2)
		// don't implicitly return the return value of the last executed instruction
		if err == signalReturn {
			out[1] = reflect.Zero(reflect.TypeOf((*error)(nil)).Elem())
		}

		switch resultKind {
		case reflect.Int:
			f, ok := result.AsFloat64()
			if !ok {
				panic("not ok")
			}
			out[0] = reflect.ValueOf(int(f))

		case reflect.Float64:
			f, ok := result.AsFloat64()
			if !ok {
				panic("not ok")
			}
			out[0] = reflect.ValueOf(f)

		default:
			panic("Call: Unsuported types returned!")
		}

		return out
	})

	reflect.ValueOf(ptr).Elem().Set(wrapper)
	return nil
}

type BoundMethod struct {
	this Value
	fn   Value
}

func (m BoundMethod) call(fbr *Fiber, arguments []instruction) (result Value, exc Exception) {
	if ufn, isUserFn := m.fn.AsUserFn(); isUserFn {
		isMethod := len(ufn.args) > 0 && ufn.args[0] == "this"
		if isMethod {
			if ufn.isVariadic {
				minArgs := len(ufn.args) - 2
				if len(arguments) < minArgs {
					return Value{}, CustomError("method '%v' requires at least %v argument(s), %v provided", ufn.name, minArgs, len(arguments))
				}
			} else {
				if len(ufn.args)-1 != len(arguments) {
					return Value{}, CustomError("method '%v' requires %v argument(s), %v provided", ufn.name, len(ufn.args)-1, len(arguments))
				}
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

			return ufn.Call(fbr, args...)
		} else {
			if ufn.isVariadic {
				minArgs := len(ufn.args) - 1
				if len(arguments) < minArgs {
					return Value{}, CustomError("function '%v' requires at least %v argument(s), %v provided", ufn.name, minArgs, len(arguments))
				}
			} else {
				if len(ufn.args) != len(arguments) {
					return Value{}, CustomError("function '%v' requires %v argument(s), %v provided", ufn.name, len(ufn.args), len(arguments))
				}
			}

			args := make([]Value, len(arguments))
			for i, arg := range arguments {
				v, exc := arg(fbr)
				if exc != nil {
					return v, exc
				}
				args[i] = v
			}

			return ufn.Call(fbr, args...)
		}
	}

	fn, ok := m.fn.AsGoFunc()
	if !ok {
		return Value{}, CustomError("property is not a callable method: %v", m.fn)
	}

	if fn.IsVariadic {
		if len(arguments) < fn.Arguments-1 {
			return Value{}, CustomError("method requires at least %v argument(s), %v provided", fn.Arguments-1, len(arguments))
		}
	} else {
		if fn.Arguments-1 != len(arguments) {
			return Value{}, CustomError("method requires %v argument(s), %v provided", fn.Arguments-1, len(arguments))
		}
	}

	base := len(fbr.stack)

	// push self
	box := fbr.pop()
	*box = m.this
	fbr.stack = append(fbr.stack, box)

	for _, arg := range arguments {
		v, exc := arg(fbr)
		if exc != nil {
			return v, exc
		}

		box := fbr.pop()
		*box = v
		fbr.stack = append(fbr.stack, box)
	}

	// save current state
	prevBase := fbr.swapBase(base)

	// call the function
	if fn.Mode == ast.UndefinedMode || fbr.synced {
		result, exc = fn.Fn(fbr)
	} else {
		fbr.vm.rt.AcquireGIL()
		fbr.synced = true
		result, exc = fn.Fn(fbr)
		fbr.synced = false
		fbr.vm.rt.ReleaseGIL()
	}

	// restore old state
	totalPushed := 1 + len(arguments)
	fbr.push(totalPushed)
	fbr.popStack(totalPushed)
	fbr.swapBase(prevBase)

	return result, exc
}

func (m BoundMethod) Call(fbr *Fiber, args ...Value) (result Value, exc Exception) {
	if ufn, isUserFn := m.fn.AsUserFn(); isUserFn {
		isMethod := len(ufn.args) > 0 && ufn.args[0] == "this"
		if isMethod {
			if ufn.isVariadic {
				minArgs := len(ufn.args) - 2
				if len(args) < minArgs {
					return Value{}, CustomError("method '%v' requires at least %v argument(s), %v provided", ufn.name, minArgs, len(args))
				}
			} else {
				if len(ufn.args)-1 != len(args) {
					return Value{}, CustomError("method '%v' requires %v argument(s), %v provided", ufn.name, len(ufn.args)-1, len(args))
				}
			}

			allArgs := make([]Value, 1+len(args))
			allArgs[0] = m.this
			copy(allArgs[1:], args)
			return ufn.Call(fbr, allArgs...)
		} else {
			if ufn.isVariadic {
				minArgs := len(ufn.args) - 1
				if len(args) < minArgs {
					return Value{}, CustomError("function '%v' requires at least %v argument(s), %v provided", ufn.name, minArgs, len(args))
				}
			} else {
				if len(ufn.args) != len(args) {
					return Value{}, CustomError("function '%v' requires %v argument(s), %v provided", ufn.name, len(ufn.args), len(args))
				}
			}

			return ufn.Call(fbr, args...)
		}
	}

	fn, ok := m.fn.AsGoFunc()
	if !ok {
		return Value{}, CustomError("property is not a callable method: %v", m.fn)
	}

	if fn.IsVariadic {
		if len(args) < fn.Arguments-1 {
			return Value{}, CustomError("method requires at least %v argument(s), %v provided", fn.Arguments-1, len(args))
		}
	} else {
		if fn.Arguments-1 != len(args) {
			return Value{}, CustomError("method requires %v argument(s), %v provided", fn.Arguments-1, len(args))
		}
	}

	base := len(fbr.stack)

	// push self
	box := fbr.pop()
	*box = m.this
	fbr.stack = append(fbr.stack, box)

	for _, v := range args {
		box := fbr.pop()
		*box = v
		fbr.stack = append(fbr.stack, box)
	}

	// save current state
	prevBase := fbr.swapBase(base)

	// call the function
	if fn.Mode == ast.UndefinedMode || fbr.synced {
		result, exc = fn.Fn(fbr)
	} else {
		fbr.vm.rt.AcquireGIL()
		fbr.synced = true
		result, exc = fn.Fn(fbr)
		fbr.synced = false
		fbr.vm.rt.ReleaseGIL()
	}

	// restore old state
	totalPushed := 1 + len(args)
	fbr.push(totalPushed)
	fbr.popStack(totalPushed)
	fbr.swapBase(prevBase)

	return result, exc
}

type GoFuncSignature = func(fbr *Fiber) (Value, Exception)

type GoFunc struct {
	Name       string
	Arguments  int
	IsVariadic bool
	IsMethod   bool
	Mode       ast.SyncMode
	Fn         GoFuncSignature
}

func (fn GoFunc) Synced() bool {
	return fn.Mode == ast.SyncedMode
}

func (fn *GoFunc) call(fbr *Fiber, arguments []instruction) (result Value, exc Exception) {
	if fn.IsVariadic {
		if len(arguments) < fn.Arguments {
			return Value{}, CustomError("callable requires at least %v argument(s), %v provided", fn.Arguments, len(arguments))
		}
	} else {
		if fn.Arguments != len(arguments) {
			return Value{}, CustomError("callable requires %v argument(s), %v provided", fn.Arguments, len(arguments))
		}
	}

	// no transition
	if fn.Mode == ast.UndefinedMode || fbr.synced {
		return fn.invoke(fbr, arguments)
	}

	// transition unsynced -> synced
	fbr.vm.rt.AcquireGIL()
	fbr.synced = true
	result, exc = fn.invoke(fbr, arguments)
	fbr.synced = false
	fbr.vm.rt.ReleaseGIL()

	return result, exc
}

// just call; no args check; no GIL consideration
func (fn *GoFunc) invoke(fbr *Fiber, arguments []instruction) (result Value, exc Exception) {
	base := len(fbr.stack)
	for _, arg := range arguments {
		v, exc := arg(fbr)
		if exc != nil {
			return v, exc
		}

		box := fbr.pop()
		*box = v
		fbr.stack = append(fbr.stack, box)
	}

	// save current state
	prevBase := fbr.swapBase(base)

	// call the fucntion
	result, exc = fn.Fn(fbr)

	// restore old state
	fbr.push(len(arguments))
	fbr.popStack(len(arguments))
	fbr.swapBase(prevBase)

	return result, exc
}

// mostly only used when calling from Go -> GoFunc
func (fn *GoFunc) Call(fbr *Fiber, args ...Value) (result Value, exc Exception) {
	if fbr == nil {
		return Value{}, CustomError("cannot call GoFunc with nil fiber")
	}

	if fn.IsVariadic {
		if len(args) < fn.Arguments {
			return Value{}, CustomError("callable requires at least %v argument(s), %v provided", fn.Arguments, len(args))
		}
	} else {
		if fn.Arguments != len(args) {
			return Value{}, CustomError("callable requires %v argument(s), %v provided", fn.Arguments, len(args))
		}
	}

	base := len(fbr.stack)
	for _, arg := range args {
		box := fbr.pop()
		*box = arg
		fbr.stack = append(fbr.stack, box)
	}

	// save current state
	prevBase := fbr.swapBase(base)

	// call the function
	if fn.Mode == ast.UndefinedMode || fbr.synced {
		result, exc = fn.Fn(fbr)
	} else {
		fbr.vm.rt.AcquireGIL()
		fbr.synced = true
		result, exc = fn.Fn(fbr)
		fbr.synced = false
		fbr.vm.rt.ReleaseGIL()
	}

	// restore old state
	fbr.push(len(args))
	fbr.popStack(len(args))
	fbr.swapBase(prevBase)

	return result, exc
}
