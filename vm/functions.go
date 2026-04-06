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

func (fn UserFn) String() string {
	return fmt.Sprintf("<function %v>", fn.name)
}

func (fn *UserFn) Call(args ...Value) (result Value, err error) {
	if len(fn.args) != len(args) {
		if fn.name != "λ" {
			return Value{}, CustomError("function '%v' requires %v argument(s), %v provided", fn.name, len(fn.args), len(args))
		}
		return Value{}, CustomError("function requires %v argument(s), %v provided", len(fn.args), len(args))
	}

	vm := fn.vm
	//vm.rt.AcquireGIL()
	//defer vm.rt.ReleaseGIL()

	// fetch a fiber and reset it
	fbr := vm.rt.fibers.Get().(*Fiber)
	fbr.synced = true
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
		if idx < len(args) {
			*fbr.stack[idx] = args[idx]
		}
	}

	// prep for execution & save currently captured values
	result, exc := fn.code(fbr)
	//fmt.Println(exc)

	// release non-escaping locals & fiber
	fbr.push(fn.recyclable)
	vm.rt.fibers.Put(fbr)

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
	fun := reflect.ValueOf(ptr).Elem()

	if len(fn.args) != fun.Type().NumIn() {
		if fn.name != "λ" {
			return CustomError("function '%v' requires %v argument(s), %v provided", fn.name, len(fn.args), fun.Type().NumIn())
		}
		return CustomError("function requires %v argument(s), %v provided", len(fn.args), fun.Type().NumIn())
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

type Method struct {
	this Value
	fn   Value
}

func (m Method) call(fbr *Fiber, arguments []instruction) (result Value, exc Exception) {
	fn, ok := m.fn.AsGoFunc()
	if !ok {
		return Value{}, RuntimeExceptionF("impossible.. how did we get here?")
	}

	if fn.Arguments-1 != len(arguments) {
		return Value{}, CustomError("method requires %v argument(s), %v provided", fn.Arguments-1, len(arguments))
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

	// call the fucntion
	result, exc = fn.Fn(fbr)

	// restore old state
	fbr.push(fn.Arguments)
	fbr.popStack(fn.Arguments)
	fbr.swapBase(prevBase)

	return result, exc
}

type GoFuncSignature = func(fbr *Fiber) (Value, Exception)

type GoFunc struct {
	Name      string
	Fn        GoFuncSignature
	Mode      ast.SyncMode
	Arguments int
	IsMethod  bool
}

func (fn GoFunc) Synced() bool {
	return fn.Mode == ast.SyncedMode
}

func (fn *GoFunc) call(fbr *Fiber, arguments []instruction) (result Value, exc Exception) {
	if fn.Arguments != len(arguments) {
		return Value{}, CustomError("function requires %v argument(s), %v provided", fn.Arguments, len(arguments))
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
	fbr.push(fn.Arguments)
	fbr.popStack(fn.Arguments)
	fbr.swapBase(prevBase)

	return result, exc
}

func (fn *GoFunc) Call(fbr *Fiber, args ...Value) (result Value, exc Exception) {
	if fn.Arguments != len(args) {
		return Value{}, CustomError("function requires %v argument(s), %v provided", fn.Arguments, len(args))
	}

	base := len(fbr.stack)
	for _, arg := range args {
		box := fbr.pop()
		*box = arg
		fbr.stack = append(fbr.stack, box)
	}

	// save current state
	prevBase := fbr.swapBase(base)

	// call the fucntion
	result, exc = fn.Fn(fbr)

	// restore old state
	fbr.push(fn.Arguments)
	fbr.popStack(fn.Arguments)
	fbr.swapBase(prevBase)

	return result, exc
}
