package vm

import (
	"fmt"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/ds"
	"github.com/hxkhan/evie/vm/fields"
)

func (vm *Instance) emitFn(node ast.Fn) instruction {
	mode := node.SyncMode
	// inherit from parent (if not specified)
	if mode == ast.UndefinedMode {
		if vm.cp.modes.Len() > 0 {
			mode = vm.cp.modes.Last(0)
		} else {
			mode = ast.SyncedMode
		}
	}
	vm.cp.modes.Push(mode)

	// create static info object
	info := &funcInfoStatic{
		name:       node.Name,
		args:       node.Args,
		isVariadic: node.IsVariadic,
		mode:       mode,
		vm:         vm,
	}

	vm.cp.closures.Push(&closure{freeVars: ds.Set[int]{}, info: info})
	vm.cp.closures.Last(0).scope.OpenBlock()

	// declare the fn arguments and only then compile the code
	for _, arg := range node.Args {
		vm.cp.closures.Last(0).scope.Declare(arg, false)
	}

	info.code = vm.compile(node.Action)
	closure := vm.cp.closures.Pop()
	vm.cp.modes.Pop()
	info.captures = closure.captures
	capacity := closure.scope.Capacity()

	// mark escapee variables
	info.locals = make([]bool, capacity)
	info.recyclable = 0
	for index := range capacity {
		if closure.freeVars.Has(index) {
			info.locals[index] = true
			vm.log.escapesf("CT: fn %v => Local(%v) escapes\n", node.Name, index)
		} else {
			info.recyclable++
		}
	}

	for i, ref := range closure.captures {
		vm.log.capturef("CT: closure => Capture(%v) -> %v\n", i, ref)
	}

	// create the function & return it
	if node.UsedAsExpr {
		return func(fbr *Fiber) (Value, Exception) {
			captured := make([]*Value, len(closure.captures))
			for i, ref := range closure.captures {
				var v *Value
				if ref.isLocal {
					v = fbr.getLocalByRef(ref.index)
				} else {
					v = fbr.getCapturedByRef(ref.index)
				}
				captured[i] = v
			}

			fn := UserFn{
				funcInfoStatic: info,
				references:     captured,
			}
			return BoxUserFn(fn), nil
		}
	}

	if node.Name == "" {
		panic("cannot declare an fn statement with no name")
	}

	index, ok := vm.cp.closures.Last(0).scope.Declare(node.Name, true)
	if !ok {
		panic(fmt.Errorf("double declaration of %s", node.Name))
	}

	// create the function & declare it
	return func(fbr *Fiber) (Value, Exception) {
		captured := make([]*Value, len(closure.captures))
		for i, ref := range closure.captures {
			var v *Value
			if ref.isLocal {
				v = fbr.getLocalByRef(ref.index)
			} else {
				v = fbr.getCapturedByRef(ref.index)
			}
			captured[i] = v
		}

		fn := UserFn{
			funcInfoStatic: info,
			references:     captured,
		}
		fbr.setLocal(index, BoxUserFn(fn))
		return Value{}, nil
	}
}

func (vm *Instance) compileStructMethod(ustruct *UserStruct, node ast.Fn) *UserFn {
	mode := node.SyncMode
	vm.cp.modes.Push(mode)

	methodArgs := make([]string, 1+len(node.Args))
	methodArgs[0] = "this"
	copy(methodArgs[1:], node.Args)

	info := &funcInfoStatic{
		name:       node.Name,
		args:       methodArgs,
		isVariadic: node.IsVariadic,
		mode:       mode,
		vm:         vm,
	}

	vm.cp.closures.Push(&closure{freeVars: ds.Set[int]{}, info: info, ustruct: ustruct})
	vm.cp.closures.Last(0).scope.OpenBlock()

	// declare arguments: "this" is declared first as a static local at slot 0
	vm.cp.closures.Last(0).scope.Declare("this", true)
	for _, arg := range node.Args {
		vm.cp.closures.Last(0).scope.Declare(arg, false)
	}

	info.code = vm.compile(node.Action)
	closure := vm.cp.closures.Pop()
	vm.cp.modes.Pop()
	info.captures = closure.captures
	capacity := closure.scope.Capacity()

	info.locals = make([]bool, capacity)
	info.recyclable = 0
	for index := range capacity {
		if closure.freeVars.Has(index) {
			info.locals[index] = true
			vm.log.escapesf("CT: method %v.%v => Local(%v) escapes\n", ustruct.Name, node.Name, index)
		} else {
			info.recyclable++
		}
	}

	return &UserFn{
		funcInfoStatic: info,
	}
}

func (vm *Instance) emitCall(node ast.Call) instruction {
	// compile arguments
	arguments := make([]instruction, len(node.Args))
	for i, arg := range node.Args {
		arguments[i] = vm.compile(arg)
	}

	// optimise: calling static functions/bindings
	if value, ok := vm.evaluate(node.Fn).(Value); ok {
		// try evie fn
		if fn, isUserFn := value.AsUserFn(); isUserFn {
			if fn.isVariadic {
				minArgs := len(fn.args) - 1
				if len(arguments) < minArgs {
					if fn.name != "λ" {
						panic(CustomError("function '%v' requires at least %v argument(s), %v provided", fn.name, minArgs, len(arguments)))
					}
					panic(CustomError("callable requires at least %v argument(s), %v provided", minArgs, len(arguments)))
				}
			} else {
				if len(fn.args) != len(arguments) {
					if fn.name != "λ" {
						panic(CustomError("function '%v' requires %v argument(s), %v provided", fn.name, len(fn.args), len(arguments)))
					}
					panic(CustomError("callable requires %v argument(s), %v provided", len(fn.args), len(arguments)))
				}
			}

			// optimise: call to ourselves (recursion)
			if vm.cp.closures.Len() > 0 && fn.funcInfoStatic == vm.cp.closures.Last(0).info {
				if !fn.isVariadic {
					return func(fbr *Fiber) (result Value, exc Exception) {
						base := len(fbr.stack)
						for _, escapes := range fn.locals {
							if !escapes {
								fbr.stack = append(fbr.stack, fbr.pop())
							} else {
								fbr.stack = append(fbr.stack, &Value{})
							}
						}

						for idx, argFn := range arguments {
							arg, exc := argFn(fbr)
							if exc != nil {
								fbr.push(fn.recyclable)
								fbr.popStack(len(fn.locals))
								return arg, exc
							}

							*(fbr.stack[base+idx]) = arg
						}

						prevBase := fbr.swapBase(base)

						if fn.mode == ast.UndefinedMode || fbr.synced {
							result, exc = fn.code(fbr)
						} else {
							vm.rt.AcquireGIL()
							fbr.synced = true
							result, exc = fn.code(fbr)
							fbr.synced = false
							vm.rt.ReleaseGIL()
						}

						fbr.push(fn.recyclable)
						fbr.popStack(len(fn.locals))
						fbr.swapBase(prevBase)

						switch exc {
						case nil:
							return Value{}, nil
						case signalReturn:
							return result, nil
						default:
							return result, exc
						}
					}
				}

				return func(fbr *Fiber) (result Value, exc Exception) {
					base := len(fbr.stack)
					for idx, escapes := range fn.locals {
						if !escapes {
							fbr.stack = append(fbr.stack, fbr.pop())
						} else {
							fbr.stack = append(fbr.stack, &Value{})
						}

						variadicIdx := len(fn.args) - 1
						if idx < variadicIdx {
							arg, exc := arguments[idx](fbr)
							if exc != nil {
								return arg, exc
							}

							*(fbr.stack[base+idx]) = arg
						} else if idx == variadicIdx {
							tailCount := len(arguments) - variadicIdx
							tail := make([]Value, tailCount)
							for t := 0; t < tailCount; t++ {
								arg, exc := arguments[variadicIdx+t](fbr)
								if exc != nil {
									return arg, exc
								}
								tail[t] = arg
							}
							*(fbr.stack[base+idx]) = BoxArray(&Array{Data: tail})
						}
					}

					prevBase := fbr.swapBase(base)

					if fn.mode == ast.UndefinedMode || fbr.synced {
						result, exc = fn.code(fbr)
					} else {
						vm.rt.AcquireGIL()
						fbr.synced = true
						result, exc = fn.code(fbr)
						fbr.synced = false
						vm.rt.ReleaseGIL()
					}

					fbr.push(fn.recyclable)
					fbr.popStack(len(fn.locals))
					fbr.swapBase(prevBase)

					switch exc {
					case nil:
						return Value{}, nil
					case signalReturn:
						return result, nil
					default:
						return result, exc
					}
				}
			}

			// optimise: call to an arbitrary static global
			if !fn.isVariadic {
				return func(fbr *Fiber) (result Value, exc Exception) {
					base := len(fbr.stack)
					for _, escapes := range fn.locals {
						if !escapes {
							fbr.stack = append(fbr.stack, fbr.pop())
						} else {
							fbr.stack = append(fbr.stack, &Value{})
						}
					}

					for idx, argFn := range arguments {
						arg, exc := argFn(fbr)
						if exc != nil {
							fbr.push(fn.recyclable)
							fbr.popStack(len(fn.locals))
							return arg, exc
						}

						*(fbr.stack[base+idx]) = arg
					}

					prevBase := fbr.swapBase(base)
					prevFn := fbr.swapActive(fn)

					if fn.mode == ast.UndefinedMode || fbr.synced {
						result, exc = fn.code(fbr)
					} else {
						vm.rt.AcquireGIL()
						fbr.synced = true
						result, exc = fn.code(fbr)
						fbr.synced = false
						vm.rt.ReleaseGIL()
					}

					fbr.push(fn.recyclable)
					fbr.popStack(len(fn.locals))
					fbr.swapBase(prevBase)
					fbr.swapActive(prevFn)

					switch exc {
					case nil:
						return Value{}, nil
					case signalReturn:
						return result, nil
					default:
						return result, exc
					}
				}
			}

			return func(fbr *Fiber) (result Value, exc Exception) {
				base := len(fbr.stack)
				for idx, escapes := range fn.locals {
					if !escapes {
						fbr.stack = append(fbr.stack, fbr.pop())
					} else {
						fbr.stack = append(fbr.stack, &Value{})
					}

					variadicIdx := len(fn.args) - 1
					if idx < variadicIdx {
						arg, exc := arguments[idx](fbr)
						if exc != nil {
							return arg, exc
						}

						*(fbr.stack[base+idx]) = arg
					} else if idx == variadicIdx {
						tailCount := len(arguments) - variadicIdx
						tail := make([]Value, tailCount)
						for t := 0; t < tailCount; t++ {
							arg, exc := arguments[variadicIdx+t](fbr)
							if exc != nil {
								return arg, exc
							}
							tail[t] = arg
						}
						*(fbr.stack[base+idx]) = BoxArray(&Array{Data: tail})
					}
				}

				prevBase := fbr.swapBase(base)
				prevFn := fbr.swapActive(fn)

				if fn.mode == ast.UndefinedMode || fbr.synced {
					result, exc = fn.code(fbr)
				} else {
					vm.rt.AcquireGIL()
					fbr.synced = true
					result, exc = fn.code(fbr)
					fbr.synced = false
					vm.rt.ReleaseGIL()
				}

				fbr.push(fn.recyclable)
				fbr.popStack(len(fn.locals))
				fbr.swapBase(prevBase)
				fbr.swapActive(prevFn)

				switch exc {
				case nil:
					return Value{}, nil
				case signalReturn:
					return result, nil
				default:
					return result, exc
				}
			}
		}

		// try go func
		if fn, isGoFunc := value.AsGoFunc(); isGoFunc {
			return func(fbr *Fiber) (Value, Exception) {
				return fn.call(fbr, arguments)
			}
		}

		// try method
		if m, isMethod := value.asMethod(); isMethod {
			return func(fbr *Fiber) (Value, Exception) {
				return m.call(fbr, arguments)
			}
		}

		// try struct
		if us, isUserStruct := value.AsUserStruct(); isUserStruct {
			if len(us.Fields) != len(arguments) {
				panic(CustomError("struct '%v' requires %v argument(s), %v provided", us.Name, len(us.Fields), len(arguments)))
			}

			numFields := len(us.Fields)
			return func(fbr *Fiber) (Value, Exception) {
				usi := &UserStructInstance{
					InstanceOf: us,
					Fields:     make([]Value, numFields),
				}

				for i := range numFields {
					v, exc := arguments[i](fbr)
					if exc != nil {
						return v, exc
					}

					usi.Fields[i] = v
				}

				return BoxUserStructInstance(usi), nil
			}
		}

		// try built-in
		if bt, isBuiltinType := value.AsBuiltinType(); isBuiltinType {
			return func(fbr *Fiber) (Value, Exception) {
				return bt.Constructor.call(fbr, arguments)
			}
		}

		panic("cannot call whatever this is")
	}

	// optimise: calling methods (avoids heap allocation of Method{})
	if iFA, ok := node.Fn.(ast.FieldAccess); ok {
		if lhs, ok := vm.evaluate(iFA.Lhs).(local); ok {
			index := fields.Get(iFA.Rhs)
			return func(fbr *Fiber) (Value, Exception) {
				obj := fbr.get(lhs)
				if pkg, ok := obj.asPackage(); ok {
					value, exists := pkg.globals[index]
					if !exists {
						return Value{}, RuntimeExceptionF("undefined symbol '%v' in '%v'", iFA.Rhs, iFA)
					}

					// try go func
					if fn, isGoFunc := value.AsGoFunc(); isGoFunc {
						return fn.call(fbr, arguments)
					}

					panic("fix.")
				}

				// 100% method
				value := obj.dotAccess(index)
				if value.IsNil() {
					return Value{}, RuntimeExceptionF("undefined symbol '%v' in '%v'", iFA.Rhs, iFA)
				}
				return BoundMethod{this: *obj, fn: value}.call(fbr, arguments)
			}
		}
	}

	// generic compilation
	value := vm.compile(node.Fn)
	return func(fbr *Fiber) (result Value, exc Exception) {
		value, exc := value(fbr)
		if exc != nil {
			return value, exc
		}

		// check if it is a user function
		if fn, isUserFn := value.AsUserFn(); isUserFn {
			if fn.isVariadic {
				minArgs := len(fn.args) - 1
				if len(arguments) < minArgs {
					if fn.name != "λ" {
						return Value{}, CustomError("function '%v' requires at least %v argument(s), %v provided", fn.name, minArgs, len(arguments))
					}
					return Value{}, CustomError("callable requires at least %v argument(s), %v provided", minArgs, len(arguments))
				}
			} else {
				if len(fn.args) != len(arguments) {
					if fn.name != "λ" {
						return Value{}, CustomError("function '%v' requires %v argument(s), %v provided", fn.name, len(fn.args), len(arguments))
					}
					return Value{}, CustomError("callable requires %v argument(s), %v provided", len(fn.args), len(arguments))
				}
			}

			// setup stack locals
			base := len(fbr.stack)
			for idx, escapes := range fn.locals {
				if !escapes {
					fbr.stack = append(fbr.stack, fbr.pop())
				} else {
					fbr.stack = append(fbr.stack, &Value{})
				}

				// evaluate arguments
				if !fn.isVariadic {
					if idx < len(arguments) {
						arg, exc := arguments[idx](fbr)
						if exc != nil {
							return arg, exc
						}

						*(fbr.stack[base+idx]) = arg
					}
				} else {
					variadicIdx := len(fn.args) - 1
					if idx < variadicIdx {
						arg, exc := arguments[idx](fbr)
						if exc != nil {
							return arg, exc
						}

						*(fbr.stack[base+idx]) = arg
					} else if idx == variadicIdx {
						tailCount := len(arguments) - variadicIdx
						tail := make([]Value, tailCount)
						for t := 0; t < tailCount; t++ {
							arg, exc := arguments[variadicIdx+t](fbr)
							if exc != nil {
								return arg, exc
							}
							tail[t] = arg
						}
						*(fbr.stack[base+idx]) = BoxArray(&Array{Data: tail})
					}
				}
			}

			// save current state
			prevBase := fbr.swapBase(base)
			prevFn := fbr.swapActive(fn)

			// no transition
			if fn.mode == ast.UndefinedMode || fbr.synced {
				result, exc = fn.code(fbr)
			} else {
				// transition unsynced -> synced
				vm.rt.AcquireGIL()
				fbr.synced = true
				result, exc = fn.code(fbr)
				fbr.synced = false
				vm.rt.ReleaseGIL()
			}

			// restore old state
			fbr.push(fn.recyclable)
			fbr.popStack(len(fn.locals))
			fbr.swapBase(prevBase)
			fbr.swapActive(prevFn)

			// return result but catch relevant signals
			switch exc {
			case nil:
				return Value{}, nil
			case signalReturn:
				return result, nil
			default:
				return result, exc
			}
		}

		// try go func
		if fn, isGoFunc := value.AsGoFunc(); isGoFunc {
			return fn.call(fbr, arguments)
		}

		// try method
		if m, isMethod := value.asMethod(); isMethod {
			return m.call(fbr, arguments)
		}

		return Value{}, CustomError("cannot call a non-function '%v'", value)
	}
}
