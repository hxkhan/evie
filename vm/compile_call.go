package vm

import (
	"unicode/utf8"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/internal/ds"
)

// compileFunction compiles a function body in a scope of its own and fills in
// the static information the frame protocol needs
func (vm *Instance) compileFunction(info *funcInfoStatic, params []string, body ast.Node, uclass *userClass) *closure {
	cl := &closure{freeVars: ds.Set[int]{}, info: info, uclass: uclass, capturedNames: findCapturedNames(body)}
	cl.scope.TrackSlots = vm.cp.debugInfo
	vm.cp.closures.Push(cl)
	scope := &cl.scope
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
		} else {
			info.recyclable++
		}
	}

	// Parameters that may escape into a nested closure are boxed into heap cells
	// right after arguments are placed in the frame.
	for i, name := range info.args {
		if closure.mayEscape(name) {
			info.escapingParams = append(info.escapingParams, i)
		}
	}

	if vm.cp.debugInfo {
		info.ensureDebug().slots = closure.scope.Slots()
	}
}

func (vm *Instance) emitFn(node ast.Fn) instruction {
	info := vm.newFuncInfo(node.Name, node.Args, node.IsVariadic, false)
	info.def = vm.at(node)

	// create the function & return it
	if node.UsedAsExpr {
		if vm.cp.debugInfo && vm.cp.closures.Len() > 0 {
			parent := vm.cp.closures.Last(0).info
			parent.ensureDebug().children = append(parent.ensureDebug().children, info)
		}
		closure := vm.compileFunction(info, node.Args, node.Action, nil)
		captures := closure.captures
		return func(fbr *fiber) (Value, exception) {
			captured := make([]*Value, len(captures))
			for i, ref := range captures {
				if ref.isLocal {
					captured[i] = fbr.getLocalByRef(ref.index)
				} else {
					captured[i] = fbr.getCapturedByRef(ref.index)
				}
			}
			return boxUserFn(userFn{funcInfoStatic: info, references: captured}), nil
		}
	}

	if node.Name == "" {
		panic(vm.compileErrorAt(node, "cannot declare an fn statement with no name"))
	}

	enclosing := vm.currentClosure()
	index, ok := enclosing.scope.Declare(node.Name, true)
	if !ok {
		panic(vm.compileErrorAt(node, "double declaration of %s", node.Name))
	}
	if vm.cp.debugInfo {
		enclosing.info.ensureDebug().children = append(enclosing.info.ensureDebug().children, info)
	}

	closure := vm.compileFunction(info, node.Args, node.Action, nil)
	captures := closure.captures
	escapes := enclosing.mayEscape(node.Name)

	// create the function & declare it
	if escapes {
		return func(fbr *fiber) (Value, exception) {
			cell := fbr.newEscapingLocal(index)
			captured := make([]*Value, len(captures))
			for i, ref := range captures {
				if ref.isLocal {
					captured[i] = fbr.getLocalByRef(ref.index)
				} else {
					captured[i] = fbr.getCapturedByRef(ref.index)
				}
			}
			*cell = boxUserFn(userFn{funcInfoStatic: info, references: captured})
			return Value{}, nil
		}
	}
	return func(fbr *fiber) (Value, exception) {
		captured := make([]*Value, len(captures))
		for i, ref := range captures {
			if ref.isLocal {
				captured[i] = fbr.getLocalByRef(ref.index)
			} else {
				captured[i] = fbr.getCapturedByRef(ref.index)
			}
		}
		fbr.setLocal(index, boxUserFn(userFn{funcInfoStatic: info, references: captured}))
		return Value{}, nil
	}
}

func (vm *Instance) compileClassMethod(uclass *userClass, node ast.Fn) *userFn {
	methodArgs := make([]string, 1+len(node.Args))
	methodArgs[0] = "this"
	copy(methodArgs[1:], node.Args)

	info := vm.newFuncInfo(node.Name, methodArgs, node.IsVariadic, true)
	info.def = vm.at(node)
	vm.compileFunction(info, methodArgs, node.Action, uclass)

	return &userFn{funcInfoStatic: info}
}

// arity panics with a compile error when a static call site passes the wrong
// number of arguments; def, when known, points at the declaration
func (vm *Instance) arity(exc exception, node ast.Call, def site) {
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
		if fn, isUserFn := value.asUserFn(); isUserFn {
			vm.arity(fn.accepts(len(arguments)), node, fn.def)

			// optimise: a direct recursive call runs on the same active function,
			// so the frame only needs to save and restore fbr.base
			if vm.isDirectSelfCall(fn) {
				return func(fbr *fiber) (result Value, exc exception) {
					if fbr.signaled() {
						if exc := fbr.halt(site); exc != nil {
							return Value{}, exc
						}
					}
					n := len(fn.locals)
					base := fbr.allocFrame(n)

					for idx, argFn := range arguments {
						arg, exc := argFn(fbr)
						if exc != nil {
							fbr.popStack(n)
							return Value{}, exc
						}
						fbr.stack[base+idx] = arg
					}
					if len(fn.escapingParams) > 0 {
						fbr.boxParams(base, fn.escapingParams)
					}

					prevBase := fbr.base
					fbr.base = base

					result, exc = fn.code(fbr)

					fbr.popStack(n)
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
				return func(fbr *fiber) (result Value, exc exception) {
					if fbr.signaled() {
						if exc := fbr.halt(site); exc != nil {
							return Value{}, exc
						}
					}
					n := len(fn.locals)
					base := fbr.allocFrame(n)

					for idx, argFn := range arguments {
						arg, exc := argFn(fbr)
						if exc != nil {
							fbr.popStack(n)
							return Value{}, exc
						}
						fbr.stack[base+idx] = arg
					}
					if len(fn.escapingParams) > 0 {
						fbr.boxParams(base, fn.escapingParams)
					}

					prevBase, prevActive := fbr.base, fbr.active
					fbr.base, fbr.active = base, fn

					result, exc = fn.code(fbr)

					fbr.popStack(n)
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

			return func(fbr *fiber) (Value, exception) {
				return fn.invoke(fbr, site, arguments)
			}
		}

		if fn, isGoFunc := value.AsGoFunc(); isGoFunc {
			vm.arity(fn.accepts(len(arguments)), node, unknownSite)
			return func(fbr *fiber) (Value, exception) {
				return fn.invoke(fbr, site, arguments)
			}
		}

		if m, isMethod := value.asMethod(); isMethod {
			return func(fbr *fiber) (Value, exception) {
				return m.call(fbr, site, arguments)
			}
		}

		if uc, isUserClass := value.asUserClass(); isUserClass {
			if uc.ParamCount != len(arguments) {
				vm.arity(customError("class '%v' takes %v argument(s) but %v were supplied", uc.Name, uc.ParamCount, len(arguments)), node, uc.def)
			}
			return func(fbr *fiber) (Value, exception) {
				return uc.construct(fbr, site, arguments)
			}
		}

		if bt, isBuiltinType := value.asBuiltinType(); isBuiltinType {
			if bt.Constructor == nil {
				panic(vm.compileErrorAt(node, "type '%s' cannot be called as a constructor", bt.Name))
			}
			vm.arity(bt.Constructor.accepts(len(arguments)), node, unknownSite)
			return func(fbr *fiber) (Value, exception) {
				return bt.Constructor.invoke(fbr, site, arguments)
			}
		}

		panic(vm.compileErrorAt(node, "cannot call '%v'", node.Fn))
	}

	// optimise: calling a member, as in `obj.method(...)` or `this.items[i].update()`
	if iFA, ok := node.Fn.(ast.FieldAccess); ok {
		if ident, ok := iFA.Lhs.(ast.Ident); ok && ident.Name == "super" {
			uclass, classLocal, dynamicClass, thisLocal := vm.resolveSuper(ident)
			methodID := fieldKeyOf(iFA.Rhs).id
			return func(fbr *fiber) (Value, exception) {
				parent := uclass.Parent
				if dynamicClass {
					if uc, ok := fbr.get(classLocal).asUserClass(); ok {
						parent = uc.Parent
					}
				}
				fn := parent.lookupMethod(methodID)
				if fn == nil {
					return Value{}, errorAt(site, "no method '%s' on parent class '%s'", iFA.Rhs, parent.Name)
				}
				if exc := fn.accepts(1 + len(arguments)); exc != nil {
					return Value{}, exc.at(site)
				}
				thisVal := *fbr.get(thisLocal)
				return fn.invokeMethod(fbr, site, thisVal, arguments)
			}
		}
		if lhs, ok := vm.evaluate(iFA.Lhs).(local); ok {
			return vm.emitMemberCall(iFA, lhs, node.Args, arguments)
		}
		return vm.emitMemberCallOn(iFA, vm.compile(iFA.Lhs), arguments)
	}

	// generic compilation: the target is a runtime value, most often a function
	if vm.cp.inline {
		if fnLocal, isLocal := vm.evaluate(node.Fn).(local); isLocal {
			return func(fbr *fiber) (Value, exception) {
				return invokeDynamic(fbr, site, *fbr.get(fnLocal), arguments)
			}
		}
	}

	value := vm.compile(node.Fn)
	return func(fbr *fiber) (Value, exception) {
		target, exc := value(fbr)
		if exc != nil {
			return target, exc
		}
		return invokeDynamic(fbr, site, target, arguments)
	}
}

// invokeDynamic calls a runtime-resolved value (most often a user closure)
// directly without going through an extra wrapper.
func invokeDynamic(fbr *fiber, site site, target Value, arguments []instruction) (Value, exception) {
	fn, isUserFn := target.asUserFn()
	if !isUserFn {
		return callValue(fbr, site, target, arguments)
	}
	if exc := fn.accepts(len(arguments)); exc != nil {
		return Value{}, exc.at(site)
	}

	// the frame protocol again, spelled out: a call through a local or a
	// closure is the second most common call shape
	if fbr.signaled() {
		if exc := fbr.halt(site); exc != nil {
			return Value{}, exc
		}
	}
	n := len(fn.locals)
	base := fbr.allocFrame(n)

	fixed := len(arguments)
	if fn.isVariadic {
		fixed = len(fn.args) - 1
	}
	for idx := range fixed {
		arg, exc := arguments[idx](fbr)
		if exc != nil {
			fbr.popStack(n)
			return Value{}, exc
		}
		fbr.stack[base+idx] = arg
	}
	if fn.isVariadic {
		if exc := fbr.evalVariadicTail(base, fixed, fixed, arguments); exc != nil {
			fbr.popStack(n)
			return Value{}, exc
		}
	}
	if len(fn.escapingParams) > 0 {
		fbr.boxParams(base, fn.escapingParams)
	}

	prevBase, prevActive := fbr.base, fbr.active
	fbr.base, fbr.active = base, fn

	result, exc := fn.code(fbr)

	fbr.popStack(n)
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

// callValue calls whatever a runtime value turns out to be with the given arguments
func callValue(fbr *fiber, site site, value Value, arguments []instruction) (Value, exception) {
	if fn, isUserFn := value.asUserFn(); isUserFn {
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
	if uc, isUserClass := value.asUserClass(); isUserClass {
		return uc.construct(fbr, site, arguments)
	}
	if bt, isBuiltinType := value.asBuiltinType(); isBuiltinType {
		if bt.Constructor == nil {
			return Value{}, errorAt(site, "type '%s' cannot be called as a constructor", bt.Name)
		}
		return bt.Constructor.call(fbr, site, arguments)
	}
	return Value{}, errorAt(site, "cannot call %s", article(value)).labelled("this is " + article(value))
}

// classMemberCache holds one resolved member (either a method or a callable
// field slot) for a fixed-shape *userClass.
type classMemberCache struct {
	class     *userClass
	method    *userFn
	fieldSlot int // >= 0 when the member is a declared field holding a callable
}

// memberSite is one `receiver.name(args)` call site. Because classes have a
// fixed shape, it caches up to four class resolutions (monomorphic hits on the
// first pointer compare, and 2..4-way polymorphic sites avoid map lookups) as
// well as immutable builtin kinds.
type memberSite struct {
	key           fieldKey
	name          string
	site          site
	c0            classMemberCache
	c1            classMemberCache
	c2            classMemberCache
	c3            classMemberCache
	cachedKind    uint64
	cachedBuiltin *GoFunc
}

func (vm *Instance) newMemberSite(iFA ast.FieldAccess) *memberSite {
	return &memberSite{
		key:  fieldKeyOf(iFA.Rhs),
		name: iFA.Rhs,
		site: vm.at(iFA),
		c0:   classMemberCache{fieldSlot: -1},
		c1:   classMemberCache{fieldSlot: -1},
		c2:   classMemberCache{fieldSlot: -1},
		c3:   classMemberCache{fieldSlot: -1},
	}
}

func (m *memberSite) invokeClassEntry(fbr *fiber, obj Value, ci *classInstance, e classMemberCache, arguments []instruction) (Value, exception) {
	if e.method != nil {
		if exc := e.method.accepts(1 + len(arguments)); exc != nil {
			return Value{}, exc.at(m.site)
		}
		return e.method.invokeMethod(fbr, m.site, obj, arguments)
	}
	if e.fieldSlot >= 0 {
		return callValue(fbr, m.site, ci.Fields[e.fieldSlot], arguments)
	}
	return Value{}, memberError(m.site, m.name, obj)
}

// call calls the member on obj. A class method is placed directly into the
// callee's frame with no bound method or argument slice; the arguments are
// evaluated by the callee, after the receiver.
func (m *memberSite) call(fbr *fiber, obj Value, arguments []instruction) (Value, exception) {
	if ci, ok := obj.asClassInstance(); ok {
		cls := ci.InstanceOf
		if cls == m.c0.class {
			return m.invokeClassEntry(fbr, obj, ci, m.c0, arguments)
		}
		if cls == m.c1.class {
			return m.invokeClassEntry(fbr, obj, ci, m.c1, arguments)
		}
		if cls == m.c2.class {
			return m.invokeClassEntry(fbr, obj, ci, m.c2, arguments)
		}
		if cls == m.c3.class {
			return m.invokeClassEntry(fbr, obj, ci, m.c3, arguments)
		}

		entry := classMemberCache{class: cls, fieldSlot: -1}
		if slot, ok := cls.slot(m.key.id); ok {
			entry.fieldSlot = slot
		} else {
			entry.method = cls.lookupMethod(m.key.id)
		}
		m.c3 = m.c2
		m.c2 = m.c1
		m.c1 = m.c0
		m.c0 = entry
		return m.invokeClassEntry(fbr, obj, ci, entry, arguments)
	}

	if pkg, ok := obj.AsPackage(); ok {
		value, exists := pkg.globals[m.key.id]
		if !exists || !value.IsPublic {
			return Value{}, memberError(m.site, m.name, obj)
		}
		return callValue(fbr, m.site, *value.Value, arguments)
	}

	if !isKnown(obj.pointer) && m.cachedBuiltin != nil && obj.scalar == m.cachedKind {
		return m.cachedBuiltin.callMethod(fbr, m.site, obj, arguments)
	}

	// builtin methods and object fields
	member := obj.dotAccess(m.key)
	if member.IsNil() {
		return Value{}, memberError(m.site, m.name, obj)
	}
	if !isKnown(obj.pointer) {
		switch obj.scalar {
		case kindArray, kindString, kindBuffer, kindTask, kindError:
			if fn, ok := member.AsGoFunc(); ok && fn.isMethod {
				m.cachedKind = obj.scalar
				m.cachedBuiltin = fn
				return fn.callMethod(fbr, m.site, obj, arguments)
			}
		}
	}
	return boundMethod{this: obj, fn: member}.call(fbr, m.site, arguments)
}

// emitMemberCall compiles `local.name(args)`
func (vm *Instance) emitMemberCall(iFA ast.FieldAccess, lhs local, argNodes []ast.Node, arguments []instruction) instruction {
	m := vm.newMemberSite(iFA)
	if vm.cp.inline {
		switch {
		case (iFA.Rhs == "push" || iFA.Rhs == "append") && len(arguments) == 1:
			switch arg := vm.evaluate(argNodes[0]).(type) {
			case Value:
				return func(fbr *fiber) (Value, exception) {
					obj := *fbr.get(lhs)
					if arr, ok := obj.AsArray(); ok {
						arr.Data = append(arr.Data, arg)
						return obj, nil
					}
					return m.call(fbr, obj, arguments)
				}
			case local:
				return func(fbr *fiber) (Value, exception) {
					obj := *fbr.get(lhs)
					if arr, ok := obj.AsArray(); ok {
						arr.Data = append(arr.Data, *fbr.get(arg))
						return obj, nil
					}
					return m.call(fbr, obj, arguments)
				}
			default:
				arg0 := arguments[0]
				return func(fbr *fiber) (Value, exception) {
					obj := *fbr.get(lhs)
					if arr, ok := obj.AsArray(); ok {
						v, exc := arg0(fbr)
						if exc != nil {
							return v, exc
						}
						arr.Data = append(arr.Data, v)
						return obj, nil
					}
					return m.call(fbr, obj, arguments)
				}
			}

		case iFA.Rhs == "pop" && len(arguments) == 0:
			return func(fbr *fiber) (Value, exception) {
				obj := *fbr.get(lhs)
				if arr, ok := obj.AsArray(); ok {
					if len(arr.Data) == 0 {
						return Value{}, nil
					}
					last := len(arr.Data) - 1
					val := arr.Data[last]
					arr.Data = arr.Data[:last]
					return val, nil
				}
				return m.call(fbr, obj, arguments)
			}

		case iFA.Rhs == "len" && len(arguments) == 0:
			return func(fbr *fiber) (Value, exception) {
				obj := *fbr.get(lhs)
				if arr, ok := obj.AsArray(); ok {
					return BoxNumber(float64(len(arr.Data))), nil
				}
				if s, ok := obj.AsString(); ok {
					return BoxNumber(float64(utf8.RuneCountInString(s))), nil
				}
				if buf, ok := obj.AsBuffer(); ok {
					return BoxNumber(float64(len(buf))), nil
				}
				return m.call(fbr, obj, arguments)
			}
		}
	}
	return func(fbr *fiber) (Value, exception) {
		return m.call(fbr, *fbr.get(lhs), arguments)
	}
}

// emitMemberCallOn compiles `expr.name(args)` for a receiver that has to be
// evaluated, such as `this.items[i].update()` or `make().run()`
func (vm *Instance) emitMemberCallOn(iFA ast.FieldAccess, receiver instruction, arguments []instruction) instruction {
	m := vm.newMemberSite(iFA)
	return func(fbr *fiber) (Value, exception) {
		obj, exc := receiver(fbr)
		if exc != nil {
			return obj, exc
		}
		return m.call(fbr, obj, arguments)
	}
}
