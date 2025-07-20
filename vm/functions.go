package vm

import (
	"fmt"
	"reflect"
)

// isLocal: lives on the stack; isCaptured: parent function is propagating the variable down to us
type capture struct {
	isLocal bool
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
	args       []string    // argument names
	captures   []capture   // captured references
	recyclable []int       // the locals that do not escape
	capacity   int         // total required scope-capacity
	code       instruction // the actual function code
	vm         *Instance   // the corresponding vm
}

type UserFn struct {
	*funcInfoStatic
	references []*Value // captured variables
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
	vm.rt.gil.Lock()
	defer vm.rt.gil.Unlock()

	// fetch a fiber and reset it
	fbr := vm.rt.fibers.Get()
	fbr.active = fn
	fbr.base = 0
	fbr.stack = fbr.stack[:0]

	// For now: just give all fibers a copy of global variables. Needs a better design.
	/* {
		fbr.stack = vm.main.stack[:len(vm.cp.globals)]
		fbr.base = len(fbr.stack)
	} */

	// create space for all the locals
	for range fn.capacity {
		fbr.pushLocal(vm.rt.boxes.Get())
	}

	// set arguments
	for idx, arg := range args {
		*fbr.stack[idx] = arg
	}

	// prep for execution & save currently captured values
	result, err = fn.code(fbr)

	// release non-escaping locals
	for _, idx := range fn.recyclable {
		vm.rt.boxes.Put(fbr.stack[idx])
	}

	// don't implicitly return the return value of the last executed instruction
	switch err {
	case nil:
		return Value{}, nil
	case errReturnSignal:
		return result, nil
	default:
		return result, err
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
		vm.rt.gil.Lock()
		defer vm.rt.gil.Unlock()

		// fetch a fiber and prepare it
		fbr := vm.rt.fibers.Get()
		fbr.active = fn
		fbr.base = 0
		fbr.stack = fbr.stack[:0]

		// create space for all the locals
		for range fn.capacity {
			fbr.pushLocal(vm.rt.boxes.Get())
		}

		// set arguments
		for idx, v := range in {
			v.Kind()
			switch v.Kind() {
			case reflect.Int, reflect.Int32, reflect.Int64:
				*fbr.stack[idx] = BoxFloat64(float64(v.Int()))
			case reflect.Float32, reflect.Float64:
				*fbr.stack[idx] = BoxFloat64(v.Float())
			case reflect.String:
				*fbr.stack[idx] = BoxString(v.String())
			default:
				panic("Call: Unsuported types supplied!")
			}
		}

		// prep for execution & save currently captured values
		result, err := fn.code(fbr)

		// release non-escaping locals
		for _, idx := range fn.recyclable {
			vm.rt.boxes.Put(fbr.stack[idx])
		}

		out = make([]reflect.Value, 2)
		// don't implicitly return the return value of the last executed instruction
		if err == errReturnSignal {
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
