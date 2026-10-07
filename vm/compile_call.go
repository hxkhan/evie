package vm

import (
	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/ds"
)

// compileFunction compiles a function body in a scope of its own and fills in
// the static information the frame protocol needs
func (vm *Instance) compileFunction(info *funcInfoStatic, params []string, body ast.Node, ustruct *UserStruct) *closure {
	vm.cp.closures.Push(&closure{freeVars: ds.Set[int]{}, info: info, ustruct: ustruct})
	scope := &vm.cp.closures.Last(0).scope
	scope.OpenBlock()

	// declare the parameters and only then compile the code; a receiver is a constant
	for i, param := range params {
		scope.Declare(param, info.isMethod && i == 0)
	}

	info.code = vm.compile(body)
	closure := vm.cp.closures.Pop()
	vm.closeFunction(info, closure)
	return closure
}

// closeFunction records what the compiled scope learned: which locals escape
// into closures and what the function captures from its parents
func (vm *Instance) closeFunction(info *funcInfoStatic, closure *closure) {
	info.captures = closure.captures
	capacity := closure.scope.Capacity()

	info.locals = make([]bool, capacity)
	info.recyclable = 0
	for index := range capacity {
		if closure.freeVars.Has(index) {
			info.locals[index] = true
			vm.log.escapesf("CT: fn %v => Local(%v) escapes\n", info.name, index)
		} else {
			info.recyclable++
		}
	}

	for i, ref := range closure.captures {
		vm.log.capturef("CT: closure => Capture(%v) -> %v\n", i, ref)
	}
}

func (vm *Instance) emitFn(node ast.Fn) instruction {
	info := vm.newFuncInfo(node.Name, node.Args, node.IsVariadic, false)
	info.def = vm.at(node)
	closure := vm.compileFunction(info, node.Args, node.Action, nil)

	captures := closure.captures

	// create the function & return it
	if node.UsedAsExpr {
		return func(fbr *Fiber) (Value, Exception) {
			captured := make([]*Value, len(captures))
			for i, ref := range captures {
				if ref.isLocal {
					captured[i] = fbr.getLocalByRef(ref.index)
				} else {
					captured[i] = fbr.getCapturedByRef(ref.index)
				}
			}
			return BoxUserFn(UserFn{funcInfoStatic: info, references: captured}), nil
		}
	}

	if node.Name == "" {
		panic(vm.compileErrorAt(node, "cannot declare an fn statement with no name"))
	}

	index, ok := vm.cp.closures.Last(0).scope.Declare(node.Name, true)
	if !ok {
		panic(vm.compileErrorAt(node, "double declaration of %s", node.Name))
	}

	// create the function & declare it
	return func(fbr *Fiber) (Value, Exception) {
		captured := make([]*Value, len(captures))
		for i, ref := range captures {
			if ref.isLocal {
				captured[i] = fbr.getLocalByRef(ref.index)
			} else {
				captured[i] = fbr.getCapturedByRef(ref.index)
			}
		}
		fbr.setLocal(index, BoxUserFn(UserFn{funcInfoStatic: info, references: captured}))
		return Value{}, nil
	}
}

func (vm *Instance) compileStructMethod(ustruct *UserStruct, node ast.Fn) *UserFn {
	methodArgs := make([]string, 1+len(node.Args))
	methodArgs[0] = "this"
	copy(methodArgs[1:], node.Args)

	info := vm.newFuncInfo(node.Name, methodArgs, node.IsVariadic, true)
	info.def = vm.at(node)
	vm.compileFunction(info, methodArgs, node.Action, ustruct)

	return &UserFn{funcInfoStatic: info}
}

// arity panics with a compile error when a static call site passes the wrong
// number of arguments; def, when known, points at the declaration
func (vm *Instance) arity(exc Exception, node ast.Call, def site) {
	if exc == nil {
		return
	}
	ce := vm.compileErrorAt(node, "%s", exc.Message)
	ce.Label = plural(len(node.Args), "argument") + " passed here"
	if def.known() {
		ce.Notes = append(ce.Notes, diag.Note{Source: def.src, Pos: def.pos, Text: "declared here"})
	}
	panic(ce)
}

func (vm *Instance) emitCall(node ast.Call) instruction {
	site := vm.at(node)
	// compile arguments
	arguments := make([]instruction, len(node.Args))
	for i, arg := range node.Args {
		arguments[i] = vm.compile(arg)
	}

	// optimise: the target is known at compile time
	if value, ok := vm.evaluate(node.Fn).(Value); ok {
		if fn, isUserFn := value.AsUserFn(); isUserFn {
			vm.arity(fn.accepts(len(arguments)), node, fn.def)

			// optimise: a call to the function being compiled runs on the same active
			// function, so the frame only needs a new base
			if !fn.isVariadic && vm.cp.closures.Len() > 0 && fn.funcInfoStatic == vm.cp.closures.Last(0).info {
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
							return Value{}, exc
						}
						*(fbr.stack[base+idx]) = arg
					}

					prevBase := fbr.base
					fbr.base = base

					result, exc = fn.code(fbr)

					fbr.push(fn.recyclable)
					fbr.popStack(len(fn.locals))
					fbr.base = prevBase

					switch exc {
					case nil:
						return Value{}, nil
					case signalReturn:
						return result, nil
					default:
						return result, exc.unwind(site)
					}
				}
			}

			if !fn.isVariadic {
				// the hot path: a static call with a fixed argument count,
				// written out in full so nothing stands between the arguments and the body
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
							return Value{}, exc
						}
						*(fbr.stack[base+idx]) = arg
					}

					prevBase, prevActive := fbr.base, fbr.active
					fbr.base, fbr.active = base, fn

					result, exc = fn.code(fbr)

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
			}

			return func(fbr *Fiber) (Value, Exception) {
				return fn.invoke(fbr, site, arguments)
			}
		}

		if fn, isGoFunc := value.AsGoFunc(); isGoFunc {
			vm.arity(fn.accepts(len(arguments)), node, unknownSite)
			return func(fbr *Fiber) (Value, Exception) {
				return fn.invoke(fbr, site, arguments)
			}
		}

		if m, isMethod := value.asMethod(); isMethod {
			return func(fbr *Fiber) (Value, Exception) {
				return m.call(fbr, site, arguments)
			}
		}

		if us, isUserStruct := value.AsUserStruct(); isUserStruct {
			if len(us.Fields) != len(arguments) {
				vm.arity(CustomError("struct '%v' takes %v argument(s) but %v were supplied", us.Name, len(us.Fields), len(arguments)), node, us.def)
			}
			return func(fbr *Fiber) (Value, Exception) {
				return us.construct(fbr, site, arguments)
			}
		}

		if bt, isBuiltinType := value.AsBuiltinType(); isBuiltinType {
			vm.arity(bt.Constructor.accepts(len(arguments)), node, unknownSite)
			return func(fbr *Fiber) (Value, Exception) {
				return bt.Constructor.invoke(fbr, site, arguments)
			}
		}

		panic(vm.compileErrorAt(node, "cannot call '%v'", node.Fn))
	}

	// optimise: calling a member of a local, as in `obj.method(...)`
	if iFA, ok := node.Fn.(ast.FieldAccess); ok {
		if lhs, ok := vm.evaluate(iFA.Lhs).(local); ok {
			return vm.emitMemberCall(iFA, lhs, arguments)
		}
	}

	// generic compilation: the target is a runtime value, most often a function
	value := vm.compile(node.Fn)
	return func(fbr *Fiber) (Value, Exception) {
		value, exc := value(fbr)
		if exc != nil {
			return value, exc
		}
		fn, isUserFn := value.AsUserFn()
		if !isUserFn {
			return callValue(fbr, site, value, arguments)
		}
		if exc := fn.accepts(len(arguments)); exc != nil {
			return Value{}, exc.at(site)
		}

		// the frame protocol again, spelled out: a call through a local or a
		// closure is the second most common call shape
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
}

// callValue calls whatever a runtime value turns out to be with the given arguments
func callValue(fbr *Fiber, site site, value Value, arguments []instruction) (Value, Exception) {
	if fn, isUserFn := value.AsUserFn(); isUserFn {
		if exc := fn.accepts(len(arguments)); exc != nil {
			return Value{}, exc.at(site)
		}
		return fn.invoke(fbr, site, arguments)
	}
	if fn, isGoFunc := value.AsGoFunc(); isGoFunc {
		return fn.call(fbr, site, arguments)
	}
	if m, isMethod := value.asMethod(); isMethod {
		return m.call(fbr, site, arguments)
	}
	if us, isUserStruct := value.AsUserStruct(); isUserStruct {
		return us.construct(fbr, site, arguments)
	}
	if bt, isBuiltinType := value.AsBuiltinType(); isBuiltinType {
		return bt.Constructor.call(fbr, site, arguments)
	}
	return Value{}, errorAt(site, "cannot call %s", article(value)).labelled("this is " + article(value))
}

// emitMemberCall compiles `local.name(args)`. The receiver is evaluated once,
// a struct method is placed directly into the callee's frame with no bound
// method or argument slice, and the method lookup is cached per call site.
func (vm *Instance) emitMemberCall(iFA ast.FieldAccess, lhs local, arguments []instruction) instruction {
	key := fieldKeyOf(iFA.Rhs)
	site := vm.at(iFA)

	// the call site remembers the last struct type it saw and the method it resolved to
	var cachedStruct *UserStruct
	var cachedMethod *UserFn

	undefined := func(obj Value) (Value, Exception) {
		return Value{}, memberError(site, iFA.Rhs, obj)
	}

	return func(fbr *Fiber) (Value, Exception) {
		obj := fbr.get(lhs)

		if usi, ok := obj.AsUserStructInstance(); ok {
			// a field holding a callable wins over a method of the same name
			if slot, ok := usi.InstanceOf.slot(key.id); ok {
				return callValue(fbr, site, usi.Fields[slot], arguments)
			}
			if usi.Dynamic != nil {
				if field, ok := usi.Dynamic[key.name]; ok {
					return callValue(fbr, site, field, arguments)
				}
			}

			if usi.InstanceOf != cachedStruct {
				cachedStruct = usi.InstanceOf
				cachedMethod = usi.InstanceOf.lookupMethod(key.id)
			}
			if cachedMethod == nil {
				return undefined(*obj)
			}
			if exc := cachedMethod.accepts(1 + len(arguments)); exc != nil {
				return Value{}, exc.at(site)
			}
			return cachedMethod.invokeMethod(fbr, site, *obj, arguments)
		}

		if pkg, ok := obj.AsPackage(); ok {
			value, exists := pkg.globals[key.id]
			if !exists {
				return undefined(*obj)
			}
			return callValue(fbr, site, *value.Value, arguments)
		}

		// builtin methods and object fields
		member := obj.dotAccess(key)
		if member.IsNil() {
			return undefined(*obj)
		}
		return BoundMethod{this: *obj, fn: member}.call(fbr, site, arguments)
	}
}
