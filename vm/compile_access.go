package vm

import (
	"math"
	"unicode/utf8"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/internal/fields"
)

func (vm *Instance) emitIdentDec(node ast.Decl) instruction {
	cl := vm.currentClosure()
	index, success := cl.scope.Reserve(node.Name, node.IsStatic)
	if !success {
		panic(vm.compileErrorAt(node, "double declaration of %s", node.Name))
	}
	escapes := cl.mayEscape(node.Name)

	// The initializer sees the bindings from before this declaration, so `x := x + 1`
	// in an inner block reads the outer x and `x := x` cannot read its own unwritten
	// slot. Only a function literal in the initializer that names the binding sees it,
	// which is how a local closure calls itself; that name is captured, so the binding
	// escapes and takes the generic path below, which creates its cell before running
	// the initializer.
	if findCapturedNames(node.Value)[node.Name] {
		cl.scope.Bind(node.Name, index, node.IsStatic)
	} else {
		defer cl.scope.Bind(node.Name, index, node.IsStatic)
	}

	// try to evaluate
	switch value := vm.evaluate(node.Value).(type) {
	case local:
		if escapes {
			return func(fbr *fiber) (Value, exception) {
				fbr.setEscapingLocal(index, *fbr.get(value))
				return Value{}, nil
			}
		}
		return func(fbr *fiber) (Value, exception) {
			fbr.setLocal(index, *fbr.get(value))
			return Value{}, nil
		}
	case Value:
		if escapes {
			return func(fbr *fiber) (Value, exception) {
				fbr.setEscapingLocal(index, value)
				return Value{}, nil
			}
		}
		return func(fbr *fiber) (Value, exception) {
			fbr.setLocal(index, value)
			return Value{}, nil
		}
	case pkgGlobal:
		if escapes {
			return func(fbr *fiber) (Value, exception) {
				fbr.setEscapingLocal(index, *value.Value)
				return Value{}, nil
			}
		}
		return func(fbr *fiber) (Value, exception) {
			fbr.setLocal(index, *value.Value)
			return Value{}, nil
		}
	}

	// Fused declaration of a binary op directly into a non-escaping local: `xx := x * x`
	if !escapes && vm.cp.inline {
		if bin, ok := node.Value.(ast.BinOp); ok {
			if fused, ok := vm.compileLocalBinAssign(local{index: int16(index)}, bin); ok {
				return fused
			}
		}
	}

	// generic
	value := vm.compile(node.Value)
	if escapes {
		return func(fbr *fiber) (Value, exception) {
			cell := fbr.newEscapingLocal(index)
			v, err := value(fbr)
			if err != nil {
				return v, err
			}
			*cell = v
			return Value{}, nil
		}
	}
	return func(fbr *fiber) (Value, exception) {
		v, err := value(fbr)
		if err != nil {
			return v, err
		}
		fbr.setLocal(index, v)
		return Value{}, nil
	}
}

func (vm *Instance) emitIdentGet(node ast.Ident) instruction {
	variable, err := vm.cp.reach(node.Name)
	if err != nil {
		panic(vm.locate(err, node))
	}

	switch v := variable.(type) {
	case local:
		if !v.isCaptured && !v.isBoxed {
			idx := int(v.index)
			return func(fbr *fiber) (Value, exception) {
				return fbr.stack[fbr.base+idx], nil
			}
		}
		return func(fbr *fiber) (Value, exception) {
			return *fbr.get(v), nil
		}
	case pkgGlobal:
		return func(fbr *fiber) (Value, exception) {
			return *v.Value, nil
		}
	}

	panic(vm.compileErrorAt(node, "cannot read '%s'", node.Name))
}

// assignField sets field name of obj to the result of value, which is only
// evaluated once obj is known to take the assignment
func assignField(fbr *fiber, site site, obj Value, name string, index fields.ID, value instruction) (Value, exception) {
	if pkg, ok := obj.AsPackage(); ok {
		field, exists := pkg.globals[index]
		if !exists || !field.IsPublic {
			return Value{}, errorAt(site, "package '%s' has no symbol '%s'", pkg.name, name)
		}
		if field.IsStatic {
			return Value{}, errorAt(site, "cannot assign to '%s.%s' because it is a constant", pkg.name, name)
		}
		v, exc := value(fbr)
		if exc != nil {
			return v, exc
		}
		*field.Value = v
		return Value{}, nil
	}

	if m, ok := obj.AsObject(); ok {
		v, exc := value(fbr)
		if exc != nil {
			return v, exc
		}
		m[name] = v
		return Value{}, nil
	}

	if ci, ok := obj.asClassInstance(); ok {
		v, exc := value(fbr)
		if exc != nil {
			return v, exc
		}
		if exc := ci.SetField(site, index, name, v); exc != nil {
			return Value{}, exc
		}
		return Value{}, nil
	}

	return Value{}, errorAt(site, "cannot assign field '%s' on %s", name, obj.TypeOf())
}

func (vm *Instance) emitAssign(node ast.Assign) instruction {
	switch lhs := node.Lhs.(type) {
	case ast.Ident:
		return vm.emitIdentAssign(lhs, node.Value)
	case ast.FieldAccess:
		return vm.emitFieldAssign(node, lhs)
	case ast.Subscript:
		return vm.emitSubscriptAssign(node, lhs)
	}
	panic(vm.compileErrorAt(node, "cannot assign to '%v'", node.Lhs))
}

func (vm *Instance) emitIdentAssign(lhs ast.Ident, rhs ast.Node) instruction {
	variable, err := vm.cp.reach(lhs.Name)
	if err != nil {
		panic(vm.locate(err, lhs))
	}

	switch v := variable.(type) {
	case local:
		if v.isStatic {
			panic(vm.compileErrorAt(lhs, "cannot assign to '%v' because it is a constant", lhs.Name).withHint("declare it with var to allow reassignment: var " + lhs.Name + " := ..."))
		}
		if vm.cp.inline {
			if bin, ok := rhs.(ast.BinOp); ok {
				if fused, ok := vm.compileLocalBinAssign(v, bin); ok {
					return fused
				}
			}
			switch val := vm.evaluate(rhs).(type) {
			case Value:
				return func(fbr *fiber) (Value, exception) {
					fbr.set(v, val)
					return Value{}, nil
				}
			case local:
				return func(fbr *fiber) (Value, exception) {
					fbr.set(v, *fbr.get(val))
					return Value{}, nil
				}
			}
		}
		value := vm.compile(rhs)
		return func(fbr *fiber) (Value, exception) {
			vNew, err := value(fbr)
			if err != nil {
				return vNew, err
			}
			fbr.set(v, vNew)
			return Value{}, nil
		}

	case pkgGlobal:
		if v.IsStatic {
			panic(vm.compileErrorAt(lhs, "cannot assign to '%v' because it is a constant", lhs.Name).withHint("declare it with var to allow reassignment: var " + lhs.Name + " := ..."))
		}
		value := vm.compile(rhs)
		return func(fbr *fiber) (Value, exception) {
			vNew, err := value(fbr)
			if err != nil {
				return vNew, err
			}
			*(v.Value) = vNew
			return Value{}, nil
		}
	}

	panic(vm.compileErrorAt(lhs, "cannot assign to '%v'", lhs.Name))
}

func (vm *Instance) emitFieldAssign(node ast.Assign, fa ast.FieldAccess) instruction {
	site := vm.at(node)
	if ident, ok := fa.Lhs.(ast.Ident); ok && ident.Name == "super" {
		panic(vm.compileErrorAt(node, "cannot assign to 'super.%s'", fa.Rhs))
	}

	switch lhs := vm.evaluate(fa.Lhs).(type) {
	case local:
		value := vm.compile(node.Value)
		index := fields.Get(fa.Rhs)

		// Fast path: assigning a declared field directly on `this` inside its own class method.
		if uclass, slot, ok := vm.thisFieldSlot(lhs, index); ok {
			if uclass.StaticFields[slot] {
				panic(vm.compileErrorAt(node, "cannot assign to constant field '%s' on class '%s'", fa.Rhs, uclass.Name).
					withHint("declare it with var to allow reassignment: var " + fa.Rhs + " := ..."))
			}
			return func(fbr *fiber) (Value, exception) {
				v, err := value(fbr)
				if err != nil {
					return v, err
				}
				thisVal := fbr.stack[fbr.base] // slot 0 of a method frame is `this`
				if ci, ok := thisVal.asClassInstance(); ok {
					if ci.InstanceOf == uclass && slot < len(ci.Fields) {
						ci.Fields[slot] = v
						return Value{}, nil
					}
					if exc := ci.SetField(site, index, fa.Rhs, v); exc != nil {
						return Value{}, exc
					}
					return Value{}, nil
				}
				return Value{}, errorAt(site, "cannot assign field '%s' on %s", fa.Rhs, thisVal.TypeOf())
			}
		}

		var cachedClass *userClass
		cachedSlot := -1
		return func(fbr *fiber) (Value, exception) {
			target := fbr.get(lhs)

			if ci, ok := target.asClassInstance(); ok {
				v, err := value(fbr)
				if err != nil {
					return v, err
				}
				if ci.InstanceOf == cachedClass && cachedSlot >= 0 {
					ci.Fields[cachedSlot] = v
					return Value{}, nil
				}
				if slot, ok := ci.InstanceOf.slot(index); ok && !ci.InstanceOf.StaticFields[slot] {
					cachedClass = ci.InstanceOf
					cachedSlot = slot
					ci.Fields[slot] = v
					return Value{}, nil
				}
				if exc := ci.SetField(site, index, fa.Rhs, v); exc != nil {
					return Value{}, exc
				}
				return Value{}, nil
			}

			if obj, ok := target.AsObject(); ok {
				v, err := value(fbr)
				if err != nil {
					return v, err
				}
				obj[fa.Rhs] = v
				return Value{}, nil
			}

			return Value{}, errorAt(site, "cannot assign field '%s' on %s", fa.Rhs, target.TypeOf())
		}

	case Value:
		// An imported package folds to a constant; verify mutability at compile time.
		if pkg, ok := lhs.AsPackage(); ok {
			field, exists := pkg.globals[fields.Get(fa.Rhs)]
			if !exists || !field.IsPublic {
				panic(vm.compileErrorAt(fa, "package '%s' has no symbol '%s'", pkg.name, fa.Rhs))
			}
			if field.IsStatic {
				panic(vm.compileErrorAt(fa, "cannot assign to '%s.%s' because it is a constant", pkg.name, fa.Rhs).withHint("declare it with var in its package to allow reassignment"))
			}

			value := vm.compile(node.Value)
			return func(fbr *fiber) (Value, exception) {
				v, err := value(fbr)
				if err != nil {
					return v, err
				}
				*(field.Value) = v
				return Value{}, nil
			}
		}

	case pkgGlobal:
		value := vm.compile(node.Value)
		index := fields.Get(fa.Rhs)
		return func(fbr *fiber) (Value, exception) {
			return assignField(fbr, site, *lhs.Value, fa.Rhs, index, value)
		}
	}

	// Any other receiver (`this.child.v = x`, `arr[i].x = v`, `make().x = v`):
	// evaluate receiver first, then the right-hand value.
	receiver := vm.compile(fa.Lhs)
	value := vm.compile(node.Value)
	index := fields.Get(fa.Rhs)
	return func(fbr *fiber) (Value, exception) {
		obj, exc := receiver(fbr)
		if exc != nil {
			return obj, exc
		}
		return assignField(fbr, site, obj, fa.Rhs, index, value)
	}
}

func assignSubscriptValue(site site, target, keyVal, newVal Value) (Value, exception) {
	// 1. Array index assignment: arr[i] = newVal
	if arr, ok := target.AsArray(); ok {
		if keyVal.pointer == f64Type {
			f := math.Float64frombits(keyVal.scalar)
			idx := int(f)
			if uint(idx) < uint(len(arr.Data)) && float64(idx) == f {
				arr.Data[idx] = newVal
				return Value{}, nil
			}
		}
		idx, ok := resolveIndex(keyVal, len(arr.Data))
		if !ok {
			return Value{}, indexError(site, keyVal, len(arr.Data), "array")
		}
		arr.Data[idx] = newVal
		return Value{}, nil
	}

	// 2. Object key assignment: obj[key] = newVal
	if obj, ok := target.AsObject(); ok {
		keyStr, ok := keyVal.AsString()
		if !ok {
			return Value{}, errorAt(site, "object key must be a string, got '%s'", keyVal.TypeOf())
		}
		obj[keyStr] = newVal
		return Value{}, nil
	}

	// 3. Class instance field assignment: classInst[key] = newVal
	if ci, ok := target.asClassInstance(); ok {
		keyStr, ok := keyVal.AsString()
		if !ok {
			return Value{}, errorAt(site, "class field key must be a string, got '%s'", keyVal.TypeOf())
		}
		if exc := ci.setFieldByName(site, keyStr, newVal); exc != nil {
			return Value{}, exc
		}
		return Value{}, nil
	}

	// 4. Buffer byte assignment: buf[i] = byteVal
	if buf, ok := target.AsBuffer(); ok {
		idx, ok := resolveIndex(keyVal, len(buf))
		if !ok {
			return Value{}, indexError(site, keyVal, len(buf), "buffer")
		}
		b, exc := checkByte(site, newVal)
		if exc != nil {
			return Value{}, exc
		}
		buf[idx] = b
		return Value{}, nil
	}

	return Value{}, errorAt(site, "cannot assign to subscript of type '%s'", target.TypeOf())
}

func (vm *Instance) emitSubscriptAssign(node ast.Assign, sub ast.Subscript) instruction {
	site := vm.at(node)

	if targetLocal, ok := vm.evaluate(sub.Lhs).(local); ok {
		tFlat := !targetLocal.isCaptured && !targetLocal.isBoxed
		ti := int(targetLocal.index)
		if keyLocal, ok := vm.evaluate(sub.Key).(local); ok {
			kFlat := !keyLocal.isCaptured && !keyLocal.isBoxed
			ki := int(keyLocal.index)
			switch val := vm.evaluate(node.Value).(type) {
			case Value:
				if tFlat && kFlat {
					return func(fbr *fiber) (Value, exception) {
						s := fbr.stack[fbr.base:]
						target, keyVal := s[ti], s[ki]
						if target.pointer != nil && !isKnown(target.pointer) {
							if target.scalar == kindArray && keyVal.pointer == f64Type {
								arr := (*Array)(target.pointer)
								f := math.Float64frombits(keyVal.scalar)
								idx := int(f)
								if uint(idx) < uint(len(arr.Data)) && float64(idx) == f {
									arr.Data[idx] = val
									return Value{}, nil
								}
							} else if target.scalar == kindObject && keyVal.scalar == kindString && !isKnown(keyVal.pointer) {
								(*(*map[string]Value)(target.pointer))[*(*string)(keyVal.pointer)] = val
								return Value{}, nil
							}
						}
						return assignSubscriptValue(site, target, keyVal, val)
					}
				}
				return func(fbr *fiber) (Value, exception) {
					target := *fbr.get(targetLocal)
					keyVal := *fbr.get(keyLocal)
					if target.pointer != nil && !isKnown(target.pointer) {
						if target.scalar == kindArray && keyVal.pointer == f64Type {
							arr := (*Array)(target.pointer)
							f := math.Float64frombits(keyVal.scalar)
							idx := int(f)
							if uint(idx) < uint(len(arr.Data)) && float64(idx) == f {
								arr.Data[idx] = val
								return Value{}, nil
							}
						} else if target.scalar == kindObject && keyVal.scalar == kindString && !isKnown(keyVal.pointer) {
							(*(*map[string]Value)(target.pointer))[*(*string)(keyVal.pointer)] = val
							return Value{}, nil
						}
					}
					return assignSubscriptValue(site, target, keyVal, val)
				}
			case local:
				if tFlat && kFlat && !val.isCaptured && !val.isBoxed {
					vi := int(val.index)
					return func(fbr *fiber) (Value, exception) {
						s := fbr.stack[fbr.base:]
						target, keyVal, newVal := s[ti], s[ki], s[vi]
						if target.pointer != nil && !isKnown(target.pointer) {
							if target.scalar == kindArray && keyVal.pointer == f64Type {
								arr := (*Array)(target.pointer)
								f := math.Float64frombits(keyVal.scalar)
								idx := int(f)
								if uint(idx) < uint(len(arr.Data)) && float64(idx) == f {
									arr.Data[idx] = newVal
									return Value{}, nil
								}
							} else if target.scalar == kindObject && keyVal.scalar == kindString && !isKnown(keyVal.pointer) {
								(*(*map[string]Value)(target.pointer))[*(*string)(keyVal.pointer)] = newVal
								return Value{}, nil
							}
						}
						return assignSubscriptValue(site, target, keyVal, newVal)
					}
				}
				return func(fbr *fiber) (Value, exception) {
					target := *fbr.get(targetLocal)
					keyVal := *fbr.get(keyLocal)
					newVal := *fbr.get(val)
					if target.pointer != nil && !isKnown(target.pointer) {
						if target.scalar == kindArray && keyVal.pointer == f64Type {
							arr := (*Array)(target.pointer)
							f := math.Float64frombits(keyVal.scalar)
							idx := int(f)
							if uint(idx) < uint(len(arr.Data)) && float64(idx) == f {
								arr.Data[idx] = newVal
								return Value{}, nil
							}
						} else if target.scalar == kindObject && keyVal.scalar == kindString && !isKnown(keyVal.pointer) {
							(*(*map[string]Value)(target.pointer))[*(*string)(keyVal.pointer)] = newVal
							return Value{}, nil
						}
					}
					return assignSubscriptValue(site, target, keyVal, newVal)
				}
			}
			valCompiled := vm.compile(node.Value)
			if tFlat && kFlat {
				return func(fbr *fiber) (Value, exception) {
					s := fbr.stack[fbr.base:]
					target, keyVal := s[ti], s[ki]
					newVal, err := valCompiled(fbr)
					if err != nil {
						return newVal, err
					}
					if target.pointer != nil && !isKnown(target.pointer) {
						if target.scalar == kindArray && keyVal.pointer == f64Type {
							arr := (*Array)(target.pointer)
							f := math.Float64frombits(keyVal.scalar)
							idx := int(f)
							if uint(idx) < uint(len(arr.Data)) && float64(idx) == f {
								arr.Data[idx] = newVal
								return Value{}, nil
							}
						} else if target.scalar == kindObject && keyVal.scalar == kindString && !isKnown(keyVal.pointer) {
							(*(*map[string]Value)(target.pointer))[*(*string)(keyVal.pointer)] = newVal
							return Value{}, nil
						}
					}
					return assignSubscriptValue(site, target, keyVal, newVal)
				}
			}
			return func(fbr *fiber) (Value, exception) {
				target := *fbr.get(targetLocal)
				keyVal := *fbr.get(keyLocal)
				newVal, err := valCompiled(fbr)
				if err != nil {
					return newVal, err
				}
				if target.pointer != nil && !isKnown(target.pointer) {
					if target.scalar == kindArray && keyVal.pointer == f64Type {
						arr := (*Array)(target.pointer)
						f := math.Float64frombits(keyVal.scalar)
						idx := int(f)
						if uint(idx) < uint(len(arr.Data)) && float64(idx) == f {
							arr.Data[idx] = newVal
							return Value{}, nil
						}
					} else if target.scalar == kindObject && keyVal.scalar == kindString && !isKnown(keyVal.pointer) {
						(*(*map[string]Value)(target.pointer))[*(*string)(keyVal.pointer)] = newVal
						return Value{}, nil
					}
				}
				return assignSubscriptValue(site, target, keyVal, newVal)
			}
		}

		keyCompiled := vm.compile(sub.Key)
		valCompiled := vm.compile(node.Value)
		return func(fbr *fiber) (Value, exception) {
			target := *fbr.get(targetLocal)
			keyVal, err := keyCompiled(fbr)
			if err != nil {
				return keyVal, err
			}
			newVal, err := valCompiled(fbr)
			if err != nil {
				return newVal, err
			}
			if target.pointer != nil && !isKnown(target.pointer) {
				if target.scalar == kindObject && keyVal.scalar == kindString && !isKnown(keyVal.pointer) {
					(*(*map[string]Value)(target.pointer))[*(*string)(keyVal.pointer)] = newVal
					return Value{}, nil
				} else if target.scalar == kindArray && keyVal.pointer == f64Type {
					arr := (*Array)(target.pointer)
					f := math.Float64frombits(keyVal.scalar)
					idx := int(f)
					if uint(idx) < uint(len(arr.Data)) && float64(idx) == f {
						arr.Data[idx] = newVal
						return Value{}, nil
					}
				}
			}
			return assignSubscriptValue(site, target, keyVal, newVal)
		}
	}

	targetCompiled := vm.compile(sub.Lhs)
	keyCompiled := vm.compile(sub.Key)
	valCompiled := vm.compile(node.Value)

	return func(fbr *fiber) (Value, exception) {
		target, err := targetCompiled(fbr)
		if err != nil {
			return target, err
		}

		keyVal, err := keyCompiled(fbr)
		if err != nil {
			return keyVal, err
		}

		newVal, err := valCompiled(fbr)
		if err != nil {
			return newVal, err
		}

		return assignSubscriptValue(site, target, keyVal, newVal)
	}
}

// resolveSuper finds the enclosing class method for a `super.method` reference
// (walking outward through nested closures so lambdas inside methods work).
//
// For top-level classes or local classes with a constant parent, uclass.Parent
// is already fixed at compile time (dynamicClass == false). For local classes
// whose parent is a local/parameter resolved at runtime (declClosure != nil),
// we also capture the local class binding from its declaring closure frame so
// each call to the enclosing function sees its own runtime parent.
func (vm *Instance) resolveSuper(node ast.Node) (uclass *userClass, classLocal local, dynamicClass bool, thisLocal local) {
	for depth := range vm.cp.closures.Len() {
		if c := vm.cp.closures.Last(depth).uclass; c != nil {
			uclass = c
			break
		}
	}
	if uclass == nil {
		panic(vm.compileErrorAt(node, "'super' can only be used inside a class method"))
	}
	if !uclass.hasParent() {
		panic(vm.compileErrorAt(node, "class '%s' has no parent class", uclass.Name))
	}
	thisBinding, err := vm.cp.reach("this")
	if err != nil {
		panic(vm.compileErrorAt(node, "'super' can only be used inside a class method"))
	}
	if uclass.declClosure != nil {
		for depth := range vm.cp.closures.Len() {
			if vm.cp.closures.Last(depth) == uclass.declClosure {
				if depth == 0 {
					classLocal = local{index: int16(uclass.declIndex), isCaptured: false, isBoxed: uclass.declClosure.mayEscape(uclass.Name), isStatic: true}
				} else {
					classLocal = local{index: int16(vm.cp.addToCaptured(depth, uclass.declIndex)), isCaptured: true, isStatic: true}
				}
				dynamicClass = true
				break
			}
		}
	}
	return uclass, classLocal, dynamicClass, thisBinding.(local)
}

func (vm *Instance) emitFieldAccess(node ast.FieldAccess) instruction {
	site := vm.at(node)
	key := fieldKeyOf(node.Rhs)
	index := key.id

	if ident, ok := node.Lhs.(ast.Ident); ok && ident.Name == "super" {
		uclass, classLocal, dynamicClass, thisLocal := vm.resolveSuper(ident)
		return func(fbr *fiber) (Value, exception) {
			parent := uclass.Parent
			if dynamicClass {
				if uc, ok := fbr.get(classLocal).asUserClass(); ok {
					parent = uc.Parent
				}
			}
			fn := parent.lookupMethod(index)
			if fn == nil {
				return Value{}, errorAt(site, "no method '%s' on parent class '%s'", node.Rhs, parent.Name)
			}
			thisVal := *fbr.get(thisLocal)
			return boxMethod(boundMethod{this: thisVal, fn: boxUserFnPtr(fn)}), nil
		}
	}

	// optimise: ident as lhs
	if lhs := vm.evaluate(node.Lhs); lhs != nil {
		switch lhs := lhs.(type) {
		case local:
			// Fast path: reading a declared field directly on `this` inside its own class method.
			if uclass, slot, ok := vm.thisFieldSlot(lhs, index); ok {
				var subClass *userClass
				subSlot := -1
				return func(fbr *fiber) (Value, exception) {
					thisVal := fbr.stack[fbr.base] // slot 0 of a method frame is `this`
					if ci, ok := thisVal.asClassInstance(); ok {
						if ci.InstanceOf == uclass {
							return ci.Fields[slot], nil
						}
						if ci.InstanceOf == subClass && subSlot >= 0 {
							return ci.Fields[subSlot], nil
						}
						if s, exists := ci.InstanceOf.slot(index); exists {
							subClass = ci.InstanceOf
							subSlot = s
							return ci.Fields[s], nil
						}
					}
					if field, exists := thisVal.getField(key); exists {
						return field, nil
					}
					return Value{}, memberError(site, node.Rhs, thisVal)
				}
			}

			var cachedClass *userClass
			var cachedSlot int = -1
			return func(fbr *fiber) (Value, exception) {
				v := fbr.get(lhs)
				if ci, ok := v.asClassInstance(); ok {
					if ci.InstanceOf == cachedClass && cachedSlot >= 0 && cachedSlot < len(ci.Fields) {
						return ci.Fields[cachedSlot], nil
					}
					if slot, ok := ci.InstanceOf.FieldOffsets[index]; ok {
						cachedClass = ci.InstanceOf
						cachedSlot = slot
						return ci.Fields[slot], nil
					}
				}
				if field, exists := v.getField(key); exists {
					return field, nil
				}
				return Value{}, memberError(site, node.Rhs, *v)
			}

		case Value:
			// a package's var, which evaluate leaves unfolded: read the binding
			// each time, since it can change after this compiles
			if pkg, ok := lhs.AsPackage(); ok {
				if global, exists := pkg.globals[key.id]; exists && global.IsPublic {
					cell := global.Value
					return func(fbr *fiber) (Value, exception) {
						return *cell, nil
					}
				}
			}
			if field, exists := lhs.getField(key); exists {
				return func(fbr *fiber) (Value, exception) {
					return field, nil
				}
			}
			panic(vm.memberCompileError(node, node.Rhs, lhs))

		case pkgGlobal:
			// global non-static binding
			return func(fbr *fiber) (Value, exception) {
				if field, exists := lhs.getField(key); exists {
					return field, nil
				}
				return Value{}, memberError(site, node.Rhs, *lhs.Value)
			}
		}
	}

	// generic compilation
	lhs := vm.compile(node.Lhs)
	var cachedClass *userClass
	var cachedSlot int = -1
	return func(fbr *fiber) (Value, exception) {
		lhs, exc := lhs(fbr)
		if exc != nil {
			return lhs, exc
		}

		if ci, ok := lhs.asClassInstance(); ok {
			if ci.InstanceOf == cachedClass && cachedSlot >= 0 && cachedSlot < len(ci.Fields) {
				return ci.Fields[cachedSlot], nil
			}
			if slot, ok := ci.InstanceOf.FieldOffsets[index]; ok {
				cachedClass = ci.InstanceOf
				cachedSlot = slot
				return ci.Fields[slot], nil
			}
		}

		if field, exists := lhs.getField(key); exists {
			return field, nil
		}

		return Value{}, memberError(site, node.Rhs, lhs)
	}
}

// resolveIndex parses an integer index (wrapping negative indices from the end)
// and reports whether it lies in [0, length). Kept free of error-path calls so
// Go inlines it at every subscript and existence-check site.
func resolveIndex(keyVal Value, length int) (int, bool) {
	idxFloat, ok := keyVal.AsFloat64()
	if !ok {
		return 0, false
	}
	idx := int(idxFloat)
	if float64(idx) != idxFloat {
		return 0, false
	}
	if idx < 0 {
		idx += length
	}
	return idx, idx >= 0 && idx < length
}

func indexError(site site, keyVal Value, length int, kind string) exception {
	idxFloat, ok := keyVal.AsFloat64()
	if !ok {
		return errorAt(site, "%s index must be a number, got '%s'", kind, keyVal.TypeOf())
	}
	idx := int(idxFloat)
	if float64(idx) != idxFloat {
		return errorAt(site, "%s index must be an integer, got %v", kind, idxFloat)
	}
	if kind == "array" {
		return errorAt(site, "IndexError: index %d out of bounds (length %d)", idx, length)
	}
	return errorAt(site, "IndexError: %s index %d out of bounds (length %d)", kind, idx, length)
}

func subscriptValue(site site, target, indexVal Value) (Value, exception) {
	// 1. Array indexing: arr[0], arr[-1]
	if arr, ok := target.AsArray(); ok {
		if indexVal.pointer == f64Type {
			f := math.Float64frombits(indexVal.scalar)
			idx := int(f)
			if uint(idx) < uint(len(arr.Data)) && float64(idx) == f {
				return arr.Data[idx], nil
			}
		}
		idx, ok := resolveIndex(indexVal, len(arr.Data))
		if !ok {
			return Value{}, indexError(site, indexVal, len(arr.Data), "array")
		}
		return arr.Data[idx], nil
	}

	// 2. Object key access: obj["key"]
	if obj, ok := target.AsObject(); ok {
		keyStr, ok := indexVal.AsString()
		if !ok {
			return Value{}, errorAt(site, "object key must be a string, got '%s'", indexVal.TypeOf())
		}
		return obj[keyStr], nil
	}

	// 3. Class instance field access: classInstance["field"]
	if ci, ok := target.asClassInstance(); ok {
		keyStr, ok := indexVal.AsString()
		if !ok {
			return Value{}, errorAt(site, "class field key must be a string, got '%s'", indexVal.TypeOf())
		}
		val, exists := ci.getFieldByName(keyStr)
		if !exists {
			return Value{}, errorAt(site, "undefined field '%s' in class", keyStr)
		}
		return val, nil
	}

	// 4. String indexing (UTF-8 runes): "héllo"[1] -> "é"
	if str, ok := target.AsString(); ok {
		idxFloat, ok := indexVal.AsFloat64()
		if !ok {
			return Value{}, indexError(site, indexVal, 0, "string")
		}
		idx := int(idxFloat)
		if float64(idx) != idxFloat {
			return Value{}, indexError(site, indexVal, 0, "string")
		}
		ch, length, ok := runeAt(str, idx)
		if !ok {
			return Value{}, indexError(site, indexVal, length, "string")
		}
		return BoxString(ch), nil
	}

	// 5. Buffer indexing: buf[0] -> byte value as number
	if buf, ok := target.AsBuffer(); ok {
		idx, ok := resolveIndex(indexVal, len(buf))
		if !ok {
			return Value{}, indexError(site, indexVal, len(buf), "buffer")
		}
		return BoxNumber(float64(buf[idx])), nil
	}

	return Value{}, errorAt(site, "cannot index value of type '%s'", target.TypeOf())
}

func (vm *Instance) emitSubscript(node ast.Subscript) instruction {
	site := vm.at(node)

	if lhsLocal, ok := vm.evaluate(node.Lhs).(local); ok {
		lFlat := !lhsLocal.isCaptured && !lhsLocal.isBoxed
		li := int(lhsLocal.index)
		switch keyOp := vm.evaluate(node.Key).(type) {
		case local:
			if lFlat && !keyOp.isCaptured && !keyOp.isBoxed {
				ki := int(keyOp.index)
				return func(fbr *fiber) (Value, exception) {
					s := fbr.stack[fbr.base:]
					target, indexVal := s[li], s[ki]
					if target.pointer != nil && !isKnown(target.pointer) {
						if target.scalar == kindArray && indexVal.pointer == f64Type {
							arr := (*Array)(target.pointer)
							f := math.Float64frombits(indexVal.scalar)
							idx := int(f)
							if uint(idx) < uint(len(arr.Data)) && float64(idx) == f {
								return arr.Data[idx], nil
							}
						} else if target.scalar == kindObject && indexVal.scalar == kindString && !isKnown(indexVal.pointer) {
							return (*(*map[string]Value)(target.pointer))[*(*string)(indexVal.pointer)], nil
						}
					}
					return subscriptValue(site, target, indexVal)
				}
			}
			return func(fbr *fiber) (Value, exception) {
				target := *fbr.get(lhsLocal)
				indexVal := *fbr.get(keyOp)
				if target.pointer != nil && !isKnown(target.pointer) {
					if target.scalar == kindArray && indexVal.pointer == f64Type {
						arr := (*Array)(target.pointer)
						f := math.Float64frombits(indexVal.scalar)
						idx := int(f)
						if uint(idx) < uint(len(arr.Data)) && float64(idx) == f {
							return arr.Data[idx], nil
						}
					} else if target.scalar == kindObject && indexVal.scalar == kindString && !isKnown(indexVal.pointer) {
						return (*(*map[string]Value)(target.pointer))[*(*string)(indexVal.pointer)], nil
					}
				}
				return subscriptValue(site, target, indexVal)
			}
		case Value:
			if keyOp.pointer == f64Type {
				f := math.Float64frombits(keyOp.scalar)
				idx := int(f)
				if idx >= 0 && float64(idx) == f {
					return func(fbr *fiber) (Value, exception) {
						target := *fbr.get(lhsLocal)
						if target.pointer != nil && !isKnown(target.pointer) && target.scalar == kindArray {
							arr := (*Array)(target.pointer)
							if idx < len(arr.Data) {
								return arr.Data[idx], nil
							}
						}
						return subscriptValue(site, target, keyOp)
					}
				}
			} else if keyStr, ok := keyOp.AsString(); ok {
				return func(fbr *fiber) (Value, exception) {
					target := *fbr.get(lhsLocal)
					if target.pointer != nil && !isKnown(target.pointer) && target.scalar == kindObject {
						return (*(*map[string]Value)(target.pointer))[keyStr], nil
					}
					return subscriptValue(site, target, keyOp)
				}
			}
		}

		key := vm.compile(node.Key)
		return func(fbr *fiber) (Value, exception) {
			target := *fbr.get(lhsLocal)
			indexVal, err := key(fbr)
			if err != nil {
				return indexVal, err
			}
			if target.pointer != nil && !isKnown(target.pointer) {
				if target.scalar == kindObject && indexVal.scalar == kindString && !isKnown(indexVal.pointer) {
					return (*(*map[string]Value)(target.pointer))[*(*string)(indexVal.pointer)], nil
				} else if target.scalar == kindArray && indexVal.pointer == f64Type {
					arr := (*Array)(target.pointer)
					f := math.Float64frombits(indexVal.scalar)
					idx := int(f)
					if uint(idx) < uint(len(arr.Data)) && float64(idx) == f {
						return arr.Data[idx], nil
					}
				}
			}
			return subscriptValue(site, target, indexVal)
		}
	}

	if keyLocal, ok := vm.evaluate(node.Key).(local); ok {
		lhs := vm.compile(node.Lhs)
		if !keyLocal.isCaptured && !keyLocal.isBoxed {
			ki := int(keyLocal.index)
			return func(fbr *fiber) (Value, exception) {
				target, err := lhs(fbr)
				if err != nil {
					return target, err
				}
				indexVal := fbr.stack[fbr.base+ki]
				if target.pointer != nil && !isKnown(target.pointer) && target.scalar == kindArray && indexVal.pointer == f64Type {
					arr := (*Array)(target.pointer)
					f := math.Float64frombits(indexVal.scalar)
					idx := int(f)
					if uint(idx) < uint(len(arr.Data)) && float64(idx) == f {
						return arr.Data[idx], nil
					}
				}
				return subscriptValue(site, target, indexVal)
			}
		}
		return func(fbr *fiber) (Value, exception) {
			target, err := lhs(fbr)
			if err != nil {
				return target, err
			}
			indexVal := *fbr.get(keyLocal)
			if target.pointer != nil && !isKnown(target.pointer) && target.scalar == kindArray && indexVal.pointer == f64Type {
				arr := (*Array)(target.pointer)
				f := math.Float64frombits(indexVal.scalar)
				idx := int(f)
				if uint(idx) < uint(len(arr.Data)) && float64(idx) == f {
					return arr.Data[idx], nil
				}
			}
			return subscriptValue(site, target, indexVal)
		}
	}

	lhs := vm.compile(node.Lhs)
	key := vm.compile(node.Key)

	return func(fbr *fiber) (Value, exception) {
		target, err := lhs(fbr)
		if err != nil {
			return target, err
		}

		indexVal, err := key(fbr)
		if err != nil {
			return indexVal, err
		}

		return subscriptValue(site, target, indexVal)
	}
}

func checkByte(site site, val Value) (byte, exception) {
	f, ok := val.AsFloat64()
	if !ok {
		return 0, errorAt(site, "buffer element must be a number, got '%s'", val.TypeOf())
	}
	b := int(f)
	if float64(b) != f || b < 0 || b > 255 {
		return 0, errorAt(site, "buffer element must be an integer in 0..255, got %v", f)
	}
	return byte(b), nil
}

func clampSlice(n, start, end int) (int, int) {
	if start < 0 {
		start = max(n+start, 0)
	} else if start > n {
		start = n
	}
	if end < 0 {
		end = max(n+end, 0)
	} else if end > n {
		end = n
	}
	if start > end {
		start = end
	}
	return start, end
}

// runeAt returns the single-rune substring at rune index idx (supporting negative
// indices) along with the string's rune length. Pure ASCII strings take an O(1) path.
func runeAt(s string, idx int) (string, int, bool) {
	n := utf8.RuneCountInString(s)
	if idx < 0 {
		idx = n + idx
	}
	if idx < 0 || idx >= n {
		return "", n, false
	}
	if n == len(s) {
		return s[idx : idx+1], n, true
	}
	pos := 0
	for i, r := range s {
		if pos == idx {
			return s[i : i+utf8.RuneLen(r)], n, true
		}
		pos++
	}
	return "", n, false
}

// sliceStringRunes slices s by clamped rune indices [start, end) without allocating
// a []rune slice; pure ASCII strings take an O(1) path.
func sliceStringRunes(s string, n, start, end int) string {
	if start >= end {
		return ""
	}
	if n == len(s) {
		return s[start:end]
	}
	byteStart, byteEnd := 0, len(s)
	pos := 0
	for i := range s {
		if pos == start {
			byteStart = i
		}
		if pos == end {
			byteEnd = i
			break
		}
		pos++
	}
	return s[byteStart:byteEnd]
}

func checkSliceBound(site site, val Value, which string) (int, exception) {
	f, ok := val.AsFloat64()
	if !ok {
		return 0, errorAt(site, "slice %s index must be a number, got '%s'", which, val.TypeOf())
	}
	idx := int(f)
	if float64(idx) != f {
		return 0, errorAt(site, "slice %s index must be an integer, got %v", which, f)
	}
	return idx, nil
}

func (vm *Instance) emitSlice(node ast.Slice) instruction {
	site := vm.at(node)
	lhs := vm.compile(node.Lhs)
	var startInst, endInst instruction
	if node.Start != nil {
		startInst = vm.compile(node.Start)
	}
	if node.End != nil {
		endInst = vm.compile(node.End)
	}

	return func(fbr *fiber) (Value, exception) {
		target, exc := lhs(fbr)
		if exc != nil {
			return target, exc
		}

		start := 0
		if startInst != nil {
			v, exc := startInst(fbr)
			if exc != nil {
				return v, exc
			}
			start, exc = checkSliceBound(site, v, "start")
			if exc != nil {
				return Value{}, exc
			}
		}

		var end int
		hasEnd := endInst != nil
		if hasEnd {
			v, exc := endInst(fbr)
			if exc != nil {
				return v, exc
			}
			end, exc = checkSliceBound(site, v, "end")
			if exc != nil {
				return Value{}, exc
			}
		}

		if arr, ok := target.AsArray(); ok {
			n := len(arr.Data)
			if !hasEnd {
				end = n
			}
			s, e := clampSlice(n, start, end)
			sub := make([]Value, e-s)
			copy(sub, arr.Data[s:e])
			return BoxArray(NewArray(sub...)), nil
		}

		if str, ok := target.AsString(); ok {
			n := utf8.RuneCountInString(str)
			if !hasEnd {
				end = n
			}
			s, e := clampSlice(n, start, end)
			return BoxString(sliceStringRunes(str, n, s, e)), nil
		}

		if buf, ok := target.AsBuffer(); ok {
			n := len(buf)
			if !hasEnd {
				end = n
			}
			s, e := clampSlice(n, start, end)
			sub := make([]byte, e-s)
			copy(sub, buf[s:e])
			return BoxBuffer(sub), nil
		}

		return Value{}, errorAt(site, "cannot slice value of type '%s'", target.TypeOf())
	}
}

func (vm *Instance) emitArrayLiteral(node ast.ArrayLiteral) instruction {
	elements := make([]instruction, len(node.Elements))
	for i, el := range node.Elements {
		elements[i] = vm.compile(el)
	}

	return func(fbr *fiber) (Value, exception) {
		values := make([]Value, len(elements))
		for i, el := range elements {
			v, err := el(fbr)
			if err != nil {
				return v, err
			}
			values[i] = v
		}
		return BoxArray(NewArray(values...)), nil
	}
}

type objectField struct {
	key   string
	value instruction
}

func (vm *Instance) emitObject(node ast.Object) instruction {
	// fields are evaluated in source order, so their side effects happen in source order
	items := make([]objectField, len(node.Fields))
	for i, f := range node.Fields {
		items[i] = objectField{key: f.Key, value: vm.compile(f.Value)}
	}

	return func(fbr *fiber) (Value, exception) {
		obj := make(map[string]Value, len(items))
		for _, item := range items {
			value, err := item.value(fbr)
			if err != nil {
				return value, err
			}

			obj[item.key] = value
		}
		return BoxObject(obj), nil
	}
}

func (vm *Instance) emitExists(node ast.Exists) instruction {
	if fa, isFieldAccess := node.Value.(ast.FieldAccess); isFieldAccess {
		value := vm.compile(fa.Lhs)
		index := fields.Get(fa.Rhs)

		return func(fbr *fiber) (Value, exception) {
			lhs, err := value(fbr)
			if err != nil {
				return lhs, err
			}

			if obj, ok := lhs.AsObject(); ok {
				_, exists := obj[fa.Rhs]
				return BoxBool(exists), nil
			}

			if obj, ok := lhs.asClassInstance(); ok {
				return BoxBool(obj.HasField(index)), nil
			}

			// nothing else has fields
			return BoxBool(false), nil
		}
	}

	if sc, isSubscript := node.Value.(ast.Subscript); isSubscript {
		target := vm.compile(sc.Lhs)
		key := vm.compile(sc.Key)
		return func(fbr *fiber) (Value, exception) {
			lhs, err := target(fbr)
			if err != nil {
				return lhs, err
			}
			k, err := key(fbr)
			if err != nil {
				return k, err
			}

			if obj, ok := lhs.AsObject(); ok {
				if keyStr, ok := k.AsString(); ok {
					_, exists := obj[keyStr]
					return BoxBool(exists), nil
				}
				return BoxBool(false), nil
			}
			if arr, ok := lhs.AsArray(); ok {
				_, ok := resolveIndex(k, len(arr.Data))
				return BoxBool(ok), nil
			}
			if str, ok := lhs.AsString(); ok {
				_, ok := resolveIndex(k, utf8.RuneCountInString(str))
				return BoxBool(ok), nil
			}
			if buf, ok := lhs.AsBuffer(); ok {
				_, ok := resolveIndex(k, len(buf))
				return BoxBool(ok), nil
			}
			if ci, ok := lhs.asClassInstance(); ok {
				if keyStr, ok := k.AsString(); ok {
					return BoxBool(ci.hasFieldByName(keyStr)), nil
				}
				return BoxBool(false), nil
			}
			return BoxBool(false), nil
		}
	}

	panic(vm.compileErrorAt(node, "'?' can only check a field or index access, such as obj.field? or arr[i]?"))
}

// compoundOperator returns the arithmetic behind a compound assignment like `+=`
func (vm *Instance) compoundOperator(node ast.MutableBinOp) func(fbr *fiber, current, rhs Value) (Value, exception) {
	site := vm.at(node)
	var apply func(Value, Value) (Value, bool)
	switch node.Operator {
	case ast.AddOp:
		apply = Value.add
	case ast.SubOp:
		apply = Value.sub
	case ast.MulOp:
		apply = Value.mul
	case ast.DivOp:
		apply = Value.div
	case ast.ModOp:
		apply = Value.mod
	default:
		panic(vm.compileErrorAt(node, "unsupported compound operator '%v'", node.Operator))
	}

	symbol := node.Operator.String()
	return func(fbr *fiber, current, rhs Value) (Value, exception) {
		if result, ok := apply(current, rhs); ok {
			return result, nil
		}
		return Value{}, operatorError(site, symbol, current, rhs)
	}
}

// emitCompoundFieldAssign compiles `x.field op= value`, evaluating `x` exactly once
func (vm *Instance) emitCompoundFieldAssign(node ast.MutableBinOp, fa ast.FieldAccess) instruction {
	site := vm.at(node)
	apply := vm.compoundOperator(node)
	index := fields.Get(fa.Rhs)

	if lhs, ok := vm.evaluate(fa.Lhs).(local); ok {
		rhs := vm.compile(node.Rhs)

		// Fast path: compound-assigning a declared field directly on `this` inside its own class method.
		if uclass, slot, ok := vm.thisFieldSlot(lhs, index); ok {
			if uclass.StaticFields[slot] {
				panic(vm.compileErrorAt(node, "cannot assign to constant field '%s' on class '%s'", fa.Rhs, uclass.Name).
					withHint("declare it with var to allow reassignment: var " + fa.Rhs + " := ..."))
			}
			return func(fbr *fiber) (Value, exception) {
				value, exc := rhs(fbr)
				if exc != nil {
					return value, exc
				}
				thisVal := fbr.stack[fbr.base]
				if ci, ok := thisVal.asClassInstance(); ok {
					if ci.InstanceOf == uclass && slot < len(ci.Fields) {
						result, exc := apply(fbr, ci.Fields[slot], value)
						if exc != nil {
							return result, exc
						}
						ci.Fields[slot] = result
						return Value{}, nil
					}
					return Value{}, ci.UpdateField(site, index, fa.Rhs, func(current Value) (Value, exception) {
						return apply(fbr, current, value)
					})
				}
				return Value{}, errorAt(site, "cannot assign field '%s' on %s", fa.Rhs, thisVal.TypeOf())
			}
		}

		var cachedClass *userClass
		cachedSlot := -1
		return func(fbr *fiber) (Value, exception) {
			obj := *fbr.get(lhs)
			value, exc := rhs(fbr)
			if exc != nil {
				return value, exc
			}
			if ci, ok := obj.asClassInstance(); ok {
				if ci.InstanceOf == cachedClass && cachedSlot >= 0 {
					result, exc := apply(fbr, ci.Fields[cachedSlot], value)
					if exc != nil {
						return result, exc
					}
					ci.Fields[cachedSlot] = result
					return Value{}, nil
				}
				if slot, ok := ci.InstanceOf.slot(index); ok && !ci.InstanceOf.StaticFields[slot] {
					cachedClass = ci.InstanceOf
					cachedSlot = slot
					result, exc := apply(fbr, ci.Fields[slot], value)
					if exc != nil {
						return result, exc
					}
					ci.Fields[slot] = result
					return Value{}, nil
				}
				return Value{}, ci.UpdateField(site, index, fa.Rhs, func(current Value) (Value, exception) {
					return apply(fbr, current, value)
				})
			}
			if m, ok := obj.AsObject(); ok {
				current, exists := m[fa.Rhs]
				if !exists {
					return Value{}, errorAt(site, "undefined key '%s' in object", fa.Rhs)
				}
				result, exc := apply(fbr, current, value)
				if exc != nil {
					return result, exc
				}
				m[fa.Rhs] = result
				return Value{}, nil
			}
			if pkg, ok := obj.AsPackage(); ok {
				field, exists := pkg.globals[index]
				if !exists || !field.IsPublic {
					return Value{}, errorAt(site, "package '%s' has no symbol '%s'", pkg.name, fa.Rhs)
				}
				if field.IsStatic {
					return Value{}, errorAt(site, "cannot assign to '%s.%s' because it is a constant", pkg.name, fa.Rhs)
				}
				result, exc := apply(fbr, *field.Value, value)
				if exc != nil {
					return result, exc
				}
				*field.Value = result
				return Value{}, nil
			}
			return Value{}, errorAt(site, "cannot assign field '%s' on %s", fa.Rhs, obj.TypeOf())
		}
	}

	target := vm.compile(fa.Lhs)
	rhs := vm.compile(node.Rhs)

	return func(fbr *fiber) (Value, exception) {
		obj, exc := target(fbr)
		if exc != nil {
			return obj, exc
		}

		value, exc := rhs(fbr)
		if exc != nil {
			return value, exc
		}

		if ci, ok := obj.asClassInstance(); ok {
			exc := ci.UpdateField(site, index, fa.Rhs, func(current Value) (Value, exception) {
				return apply(fbr, current, value)
			})
			return Value{}, exc
		}

		if m, ok := obj.AsObject(); ok {
			current, exists := m[fa.Rhs]
			if !exists {
				return Value{}, errorAt(site, "undefined key '%s' in object", fa.Rhs)
			}
			result, exc := apply(fbr, current, value)
			if exc != nil {
				return result, exc
			}
			m[fa.Rhs] = result
			return Value{}, nil
		}

		if pkg, ok := obj.AsPackage(); ok {
			field, exists := pkg.globals[index]
			if !exists || !field.IsPublic {
				return Value{}, errorAt(site, "package '%s' has no symbol '%s'", pkg.name, fa.Rhs)
			}
			if field.IsStatic {
				return Value{}, errorAt(site, "cannot assign to '%s.%s' because it is a constant", pkg.name, fa.Rhs)
			}
			result, exc := apply(fbr, *field.Value, value)
			if exc != nil {
				return result, exc
			}
			*field.Value = result
			return Value{}, nil
		}

		return Value{}, errorAt(site, "cannot assign field '%s' on %s", fa.Rhs, obj.TypeOf())
	}
}

// emitCompoundSubscriptAssign compiles `x[key] op= value`, evaluating `x` and `key` exactly once
func (vm *Instance) emitCompoundSubscriptAssign(node ast.MutableBinOp, sub ast.Subscript) instruction {
	site := vm.at(node)
	target := vm.compile(sub.Lhs)
	key := vm.compile(sub.Key)
	rhs := vm.compile(node.Rhs)
	apply := vm.compoundOperator(node)

	return func(fbr *fiber) (Value, exception) {
		obj, exc := target(fbr)
		if exc != nil {
			return obj, exc
		}

		keyVal, exc := key(fbr)
		if exc != nil {
			return keyVal, exc
		}

		value, exc := rhs(fbr)
		if exc != nil {
			return value, exc
		}

		if arr, ok := obj.AsArray(); ok {
			idx, ok := resolveIndex(keyVal, len(arr.Data))
			if !ok {
				return Value{}, indexError(site, keyVal, len(arr.Data), "array")
			}
			result, exc := apply(fbr, arr.Data[idx], value)
			if exc != nil {
				return result, exc
			}
			arr.Data[idx] = result
			return Value{}, nil
		}

		keyStr, isString := keyVal.AsString()

		if m, ok := obj.AsObject(); ok {
			if !isString {
				return Value{}, errorAt(site, "object key must be a string, got '%s'", keyVal.TypeOf())
			}
			current, exists := m[keyStr]
			if !exists {
				return Value{}, errorAt(site, "undefined key '%s' in object", keyStr)
			}
			result, exc := apply(fbr, current, value)
			if exc != nil {
				return result, exc
			}
			m[keyStr] = result
			return Value{}, nil
		}

		if ci, ok := obj.asClassInstance(); ok {
			if !isString {
				return Value{}, errorAt(site, "class field key must be a string, got '%s'", keyVal.TypeOf())
			}
			exc := ci.updateFieldByName(site, keyStr, func(current Value) (Value, exception) {
				return apply(fbr, current, value)
			})
			return Value{}, exc
		}

		if buf, ok := obj.AsBuffer(); ok {
			idx, ok := resolveIndex(keyVal, len(buf))
			if !ok {
				return Value{}, indexError(site, keyVal, len(buf), "buffer")
			}
			result, exc := apply(fbr, BoxNumber(float64(buf[idx])), value)
			if exc != nil {
				return result, exc
			}
			b, exc := checkByte(site, result)
			if exc != nil {
				return Value{}, exc
			}
			buf[idx] = b
			return Value{}, nil
		}

		return Value{}, errorAt(site, "cannot assign to subscript of type '%s'", obj.TypeOf())
	}
}
