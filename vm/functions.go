package vm

import (
	"fmt"
	"reflect"
	"unsafe"

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
	return "<function>"
}

func (fn *UserFn) Call(args ...Value) (result Value, err error) {
	if len(fn.args) != len(args) {
		if fn.name != "λ" {
			return Value{}, CustomError("function '%v' requires %v argument(s), %v provided", fn.name, len(fn.args), len(args))
		}
		return Value{}, CustomError("function requires %v argument(s), %v provided", len(fn.args), len(args))
	}

	vm := fn.vm
	vm.rt.AcquireGIL()
	defer vm.rt.ReleaseGIL()

	// fetch a fiber and reset it
	fbr := vm.rt.fibers.Get().(*fiber)
	fbr.unsynchronized = false
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
	case returnSignal:
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
		fbr := vm.rt.fibers.Get().(*fiber)
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
		if err == returnSignal {
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

func (m Method) call(fbr *fiber, arguments []instruction) (result Value, exc *Exception) {
	fn, ok := m.fn.AsGoFunc()
	if !ok {
		return Value{}, notFunction
	}

	if fn.nargs-1 != len(arguments) {
		return Value{}, CustomError("method requires %v argument(s), %v provided", fn.nargs-1, len(arguments))
	}

	synced := fn.Synced()
	switch {
	// no transition
	case fn.mode == ast.AgnosticMode || fbr.synced() == synced:
		break

	// to unsynced
	case !synced:
		fbr.vm.rt.ReleaseGIL()
		defer fbr.vm.rt.AcquireGIL()

	// to synced
	default:
		fbr.vm.rt.AcquireGIL()
		defer fbr.vm.rt.ReleaseGIL()
	}

	switch fn.nargs {
	case -1:
		panic("variadic functions not supported yet")
	case 0:
		panic("how did we get a method that does not even take itself as an arguement?")
	case 1:
		function := *(*func(Value) (Value, *Exception))(fn.ptr)
		return function(m.this)
	case 2:
		function := *(*func(Value, Value) (Value, *Exception))(fn.ptr)
		arg0, err := arguments[0](fbr)
		if err != nil {
			return arg0, err
		}
		return function(m.this, arg0)
	}

	panic("unsuported call")
}

type GoFunc struct {
	nargs int
	ptr   unsafe.Pointer
	mode  ast.SyncMode
}

func (fn GoFunc) Synced() bool {
	return fn.mode == ast.SyncedMode
}

func (fn *GoFunc) call(fbr *fiber, arguments []instruction) (result Value, exc *Exception) {
	if fn.nargs != len(arguments) {
		return Value{}, CustomError("function requires %v argument(s), %v provided", fn.nargs, len(arguments))
	}

	synced := fn.Synced()
	switch {
	// no transition
	case fn.mode == ast.AgnosticMode || fbr.synced() == synced:
		break

	// to unsynced
	case !synced:
		fbr.vm.rt.ReleaseGIL()
		defer fbr.vm.rt.AcquireGIL()

	// to synced
	default:
		fbr.vm.rt.AcquireGIL()
		defer fbr.vm.rt.ReleaseGIL()
	}

	switch fn.nargs {
	case -1:
		panic("variadic functions not supported yet")
	case 0:
		function := *(*func() (Value, *Exception))(fn.ptr)
		return function()
	case 1:
		function := *(*func(Value) (Value, *Exception))(fn.ptr)
		arg0, err := arguments[0](fbr)
		if err != nil {
			return arg0, err
		}
		return function(arg0)
	case 2:
		function := *(*func(Value, Value) (Value, *Exception))(fn.ptr)
		arg0, err := arguments[0](fbr)
		if err != nil {
			return arg0, err
		}

		arg1, err := arguments[1](fbr)
		if err != nil {
			return arg0, err
		}
		return function(arg0, arg1)
	}

	panic("unsuported call")
}

func (fn *GoFunc) jcall(fbr *fiber, arguments []instruction) (result Value, exc *Exception) {
	switch fn.nargs {
	case -1:
		panic("variadic functions not supported yet")
	case 0:
		function := *(*func() (Value, *Exception))(fn.ptr)
		return function()
	case 1:
		function := *(*func(Value) (Value, *Exception))(fn.ptr)
		arg0, err := arguments[0](fbr)
		if err != nil {
			return arg0, err
		}
		return function(arg0)
	case 2:
		function := *(*func(Value, Value) (Value, *Exception))(fn.ptr)
		arg0, err := arguments[0](fbr)
		if err != nil {
			return arg0, err
		}

		arg1, err := arguments[1](fbr)
		if err != nil {
			return arg0, err
		}
		return function(arg0, arg1)
	}

	panic("unsuported call")
}
