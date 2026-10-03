package vm

import (
	"fmt"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/vm/fields"
)

func (vm *Instance) emitIdentDec(node ast.Decl) instruction {
	index, success := vm.cp.closures.Last(0).scope.Declare(node.Name, node.IsStatic)
	if !success {
		panic(vm.compileErrorAt(node, "double declaration of %s", node.Name))
	}

	// try to evaluate
	switch value := vm.evaluate(node.Value).(type) {
	case local:
		return func(fbr *Fiber) (Value, Exception) {
			fbr.setLocal(index, *fbr.get(value))
			return Value{}, nil
		}
	case Value:
		return func(fbr *Fiber) (Value, Exception) {
			fbr.setLocal(index, value)
			return Value{}, nil
		}
	case Global:
		return func(fbr *Fiber) (Value, Exception) {
			fbr.setLocal(index, *value.Value)
			return Value{}, nil
		}
	}

	// generic
	value := vm.compile(node.Value)
	return func(fbr *Fiber) (Value, Exception) {
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
		return func(fbr *Fiber) (Value, Exception) {
			return *fbr.get(v), nil
		}
	case Global:
		return func(fbr *Fiber) (Value, Exception) {
			return *v.Value, nil
		}
	}

	panic(vm.compileErrorAt(node, "cannot read '%s'", node.Name))
}

// assignField sets field name of obj to the result of value, which is only
// evaluated once obj is known to take the assignment
func assignField(fbr *Fiber, site site, obj Value, name string, index fields.ID, value instruction) (Value, Exception) {
	if pkg, ok := obj.AsPackage(); ok {
		field, exists := pkg.globals[index]
		if !exists {
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

	if usi, ok := obj.AsUserStructInstance(); ok {
		v, exc := value(fbr)
		if exc != nil {
			return v, exc
		}
		usi.SetField(index, v)
		return Value{}, nil
	}

	return Value{}, errorAt(site, "cannot assign field '%s' on %s", name, obj.TypeOf())
}

func (vm *Instance) emitAssign(node ast.Assign) instruction {
	site := vm.at(node)
	// handle variables
	if iGet, isIdentGet := node.Lhs.(ast.Ident); isIdentGet {
		variable, err := vm.cp.reach(iGet.Name)
		if err != nil {
			panic(vm.locate(err, iGet))
		}

		// compile new value
		value := vm.compile(node.Value)

		switch v := variable.(type) {
		case local:
			if v.isStatic {
				panic(vm.compileErrorAt(iGet, "cannot assign to '%v' because it is a constant", iGet.Name).withHint("declare it with var to allow reassignment: var " + iGet.Name + " := ..."))
			}

			return func(fbr *Fiber) (Value, Exception) {
				value, err := value(fbr)
				if err != nil {
					return value, err
				}

				fbr.set(v, value)
				return Value{}, nil
			}

		case Global:
			if v.IsStatic {
				panic(vm.compileErrorAt(iGet, "cannot assign to '%v' because it is a constant", iGet.Name).withHint("declare it with var to allow reassignment: var " + iGet.Name + " := ..."))
			}

			return func(fbr *Fiber) (Value, Exception) {
				value, err := value(fbr)
				if err != nil {
					return value, err
				}

				*(v.Value) = value
				return Value{}, nil
			}

		}
	}

	// handle field access assignments
	if fa, isFieldAccess := node.Lhs.(ast.FieldAccess); isFieldAccess {
		// Lhs can for e.g. be an ast.Ident
		switch lhs := vm.evaluate(fa.Lhs).(type) {
		case local:
			// compile new value & return setter
			value := vm.compile(node.Value)
			index := fields.Get(fa.Rhs)

			if lhs.index == 0 && !lhs.isCaptured && vm.cp.closures.Len() > 0 && vm.cp.closures.Last(0).ustruct != nil {
				ustruct := vm.cp.closures.Last(0).ustruct
				if slot, ok := ustruct.FieldOffsets[index]; ok {
					return func(fbr *Fiber) (Value, Exception) {
						v, err := value(fbr)
						if err != nil {
							return v, err
						}
						thisVal := fbr.stack[fbr.base]
						if thisVal.scalar == kindStructInstance && !isKnown(thisVal.pointer) {
							usi := (*UserStructInstance)(thisVal.pointer)
							if usi.InstanceOf == ustruct && slot < len(usi.Fields) {
								usi.Fields[slot] = v
								return Value{}, nil
							}
							usi.SetField(index, v)
							return Value{}, nil
						}
						return Value{}, errorAt(site, "cannot assign field '%s' on %s", fa.Rhs, thisVal.TypeOf())
					}
				}
			}

			return func(fbr *Fiber) (Value, Exception) {
				lhs := fbr.get(lhs)

				if obj, ok := lhs.AsObject(); ok {
					value, err := value(fbr)
					if err != nil {
						return value, err
					}

					obj[fa.Rhs] = value
					return Value{}, nil
				}

				if obj, ok := lhs.AsUserStructInstance(); ok {
					value, err := value(fbr)
					if err != nil {
						return value, err
					}

					obj.SetField(index, value)
					return Value{}, nil
				}

				return Value{}, errorAt(site, "cannot assign field '%s' on %s", fa.Rhs, lhs.TypeOf())
			}

		case Value:
			// an imported package folds to a constant; its symbols are checked now
			if pkg, ok := lhs.AsPackage(); ok {
				field, exists := pkg.globals[fields.Get(fa.Rhs)]
				if !exists {
					panic(vm.compileErrorAt(fa, "package '%s' has no symbol '%s'", pkg.name, fa.Rhs))
				}

				if field.IsStatic {
					panic(vm.compileErrorAt(fa, "cannot assign to '%s.%s' because it is a constant", pkg.name, fa.Rhs).withHint("declare it with var in its package to allow reassignment"))
				}

				value := vm.compile(node.Value)
				return func(fbr *Fiber) (Value, Exception) {
					value, err := value(fbr)
					if err != nil {
						return value, err
					}

					*(field.Value) = value
					return Value{}, nil
				}
			}

		case Global:
			// compile new value & return setter
			value := vm.compile(node.Value)
			index := fields.Get(fa.Rhs)
			return func(fbr *Fiber) (Value, Exception) {
				if pkg, ok := lhs.AsPackage(); ok {
					field, exists := pkg.globals[index]
					if !exists {
						return Value{}, errorAt(site, "package '%s' has no symbol '%s'", pkg.name, fa.Rhs)
					}

					if field.IsStatic {
						return Value{}, errorAt(site, "cannot assign to '%s.%s' because it is a constant", pkg.name, fa.Rhs)
					}

					value, err := value(fbr)
					if err != nil {
						return value, err
					}

					*(field.Value) = value
					return Value{}, nil
				}

				if obj, ok := lhs.AsObject(); ok {
					value, err := value(fbr)
					if err != nil {
						return value, err
					}

					obj[fa.Rhs] = value
					return Value{}, nil
				}

				if obj, ok := lhs.AsUserStructInstance(); ok {
					value, err := value(fbr)
					if err != nil {
						return value, err
					}

					obj.SetField(index, value)
					return Value{}, nil
				}

				return Value{}, errorAt(site, "cannot assign field '%s' on %s", fa.Rhs, lhs.TypeOf())
			}
		}

		// any other receiver, as in `this.child.v = x`, `arr[i].x = v` or
		// `make().x = v`: the receiver is evaluated first, then the value
		receiver := vm.compile(fa.Lhs)
		value := vm.compile(node.Value)
		index := fields.Get(fa.Rhs)
		return func(fbr *Fiber) (Value, Exception) {
			obj, exc := receiver(fbr)
			if exc != nil {
				return obj, exc
			}
			return assignField(fbr, site, obj, fa.Rhs, index, value)
		}
	}

	// handle subscript assignments: arr[i] = val, obj[key] = val, structInstance[key] = val
	if sub, isSubscript := node.Lhs.(ast.Subscript); isSubscript {
		targetCompiled := vm.compile(sub.Lhs)
		keyCompiled := vm.compile(sub.Key)
		valCompiled := vm.compile(node.Value)

		return func(fbr *Fiber) (Value, Exception) {
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

			// 1. Array index assignment: arr[i] = newVal
			if arr, ok := target.AsArray(); ok {
				idxFloat, ok := keyVal.AsFloat64()
				if !ok {
					return Value{}, errorAt(site, "array index must be a number, got '%s'", keyVal.TypeOf())
				}
				idx := int(idxFloat)
				if float64(idx) != idxFloat {
					return Value{}, errorAt(site, "array index must be an integer, got %v", idxFloat)
				}
				length := len(arr.Data)
				if idx < 0 {
					idx = length + idx
				}

				if idx < 0 || idx >= length {
					return Value{}, errorAt(site, "IndexError: index %d out of bounds (length %d)", int(idxFloat), length)
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

			// 3. Struct instance field assignment: structInst[key] = newVal
			if si, ok := target.AsUserStructInstance(); ok {
				keyStr, ok := keyVal.AsString()
				if !ok {
					return Value{}, errorAt(site, "struct field key must be a string, got '%s'", keyVal.TypeOf())
				}

				si.setFieldByName(keyStr, newVal)
				return Value{}, nil
			}

			return Value{}, errorAt(site, "cannot assign to subscript of type '%s'", target.TypeOf())
		}
	}

	panic(vm.compileErrorAt(node, "cannot assign to '%v'", node.Lhs))
}

func (vm *Instance) emitFieldAccess(node ast.FieldAccess) instruction {
	site := vm.at(node)
	key := fieldKeyOf(node.Rhs)
	index := key.id

	// optimise: ident as lhs
	if lhs := vm.evaluate(node.Lhs); lhs != nil {
		switch lhs := lhs.(type) {
		case local:
			if lhs.index == 0 && !lhs.isCaptured && vm.cp.closures.Len() > 0 && vm.cp.closures.Last(0).ustruct != nil {
				ustruct := vm.cp.closures.Last(0).ustruct
				if slot, ok := ustruct.FieldOffsets[index]; ok {
					return func(fbr *Fiber) (Value, Exception) {
						thisVal := fbr.stack[fbr.base]
						if thisVal.scalar == kindStructInstance && !isKnown(thisVal.pointer) {
							usi := (*UserStructInstance)(thisVal.pointer)
							if usi.InstanceOf == ustruct && slot < len(usi.Fields) {
								val := usi.Fields[slot]
								return val, nil
							}
							if val, exists := usi.GetField(index); exists {
								return val, nil
							}
						}
						if field, exists := thisVal.getField(key); exists {
							return field, nil
						}
						return Value{}, memberError(site, node.Rhs, *thisVal)
					}
				}
			}

			var cachedStruct *UserStruct
			var cachedSlot int = -1
			return func(fbr *Fiber) (Value, Exception) {
				v := fbr.get(lhs)
				if v.scalar == kindStructInstance && !isKnown(v.pointer) {
					usi := (*UserStructInstance)(v.pointer)
					if usi.InstanceOf == cachedStruct && cachedSlot >= 0 && cachedSlot < len(usi.Fields) {
						val := usi.Fields[cachedSlot]
						return val, nil
					}
					if slot, ok := usi.InstanceOf.FieldOffsets[index]; ok {
						cachedStruct = usi.InstanceOf
						cachedSlot = slot
						val := usi.Fields[slot]
						return val, nil
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
					return func(fbr *Fiber) (Value, Exception) {
						return *cell, nil
					}
				}
			}
			if field, exists := lhs.getField(key); exists {
				return func(fbr *Fiber) (Value, Exception) {
					return field, nil
				}
			}
			panic(vm.memberCompileError(node, node.Rhs, lhs))

		case Global:
			// global non-static binding
			return func(fbr *Fiber) (Value, Exception) {
				if field, exists := lhs.getField(key); exists {
					return field, nil
				}
				return Value{}, memberError(site, node.Rhs, *lhs.Value)
			}
		}
	}

	// generic compilation
	lhs := vm.compile(node.Lhs)
	var cachedStruct *UserStruct
	var cachedSlot int = -1
	return func(fbr *Fiber) (Value, Exception) {
		lhs, exc := lhs(fbr)
		if exc != nil {
			return lhs, exc
		}

		if lhs.scalar == kindStructInstance && !isKnown(lhs.pointer) {
			usi := (*UserStructInstance)(lhs.pointer)
			if usi.InstanceOf == cachedStruct && cachedSlot >= 0 && cachedSlot < len(usi.Fields) {
				val := usi.Fields[cachedSlot]
				return val, nil
			}
			if slot, ok := usi.InstanceOf.FieldOffsets[index]; ok {
				cachedStruct = usi.InstanceOf
				cachedSlot = slot
				val := usi.Fields[slot]
				return val, nil
			}
		}

		if field, exists := lhs.getField(key); exists {
			return field, nil
		}

		return Value{}, memberError(site, node.Rhs, lhs)
	}
}

func (vm *Instance) emitSubscript(node ast.Subscript) instruction {
	site := vm.at(node)
	lhs := vm.compile(node.Lhs)
	key := vm.compile(node.Key)

	return func(fbr *Fiber) (Value, Exception) {
		target, err := lhs(fbr)
		if err != nil {
			return target, err
		}

		indexVal, err := key(fbr)
		if err != nil {
			return indexVal, err
		}

		// 1. Array indexing: arr[0], arr[-1]
		if arr, ok := target.AsArray(); ok {
			idxFloat, ok := indexVal.AsFloat64()
			if !ok {
				return Value{}, errorAt(site, "array index must be a number, got '%s'", indexVal.TypeOf())
			}
			idx := int(idxFloat)
			if float64(idx) != idxFloat {
				return Value{}, errorAt(site, "array index must be an integer, got %v", idxFloat)
			}
			length := len(arr.Data)
			if idx < 0 {
				idx = length + idx
			}

			if idx < 0 || idx >= length {
				return Value{}, errorAt(site, "IndexError: index %d out of bounds (length %d)", int(idxFloat), length)
			}

			return arr.Data[idx], nil
		}

		// 2. Object key access: obj["key"]
		if obj, ok := target.AsObject(); ok {
			keyStr, ok := indexVal.AsString()
			if !ok {
				return Value{}, errorAt(site, "object key must be a string, got '%s'", indexVal.TypeOf())
			}

			val, exists := obj[keyStr]
			if !exists {
				return Value{}, nil
			}
			return val, nil
		}

		// 3. Struct instance field access: structInstance["field"]
		if si, ok := target.AsUserStructInstance(); ok {
			keyStr, ok := indexVal.AsString()
			if !ok {
				return Value{}, errorAt(site, "struct field key must be a string, got '%s'", indexVal.TypeOf())
			}

			val, exists := si.getFieldByName(keyStr)
			if !exists {
				return Value{}, errorAt(site, "undefined field '%s' in struct", keyStr)
			}
			return val, nil
		}

		// 4. String indexing: "hello"[1] -> "e"
		if str, ok := target.AsString(); ok {
			idxFloat, ok := indexVal.AsFloat64()
			if !ok {
				return Value{}, errorAt(site, "string index must be a number, got '%s'", indexVal.TypeOf())
			}
			idx := int(idxFloat)
			if float64(idx) != idxFloat {
				return Value{}, errorAt(site, "string index must be an integer, got %v", idxFloat)
			}

			length := len(str)
			if idx < 0 {
				idx = length + idx
			}

			if idx < 0 || idx >= length {
				return Value{}, errorAt(site, "IndexError: string index %d out of bounds (length %d)", int(idxFloat), length)
			}

			return BoxString(string(str[idx])), nil
		}

		// 5. Buffer indexing: buf[0] -> byte value as number
		if buf, ok := target.AsBuffer(); ok {
			idxFloat, ok := indexVal.AsFloat64()
			if !ok {
				return Value{}, errorAt(site, "buffer index must be a number, got '%s'", indexVal.TypeOf())
			}
			idx := int(idxFloat)
			if float64(idx) != idxFloat {
				return Value{}, errorAt(site, "buffer index must be an integer, got %v", idxFloat)
			}

			length := len(buf)
			if idx < 0 {
				idx = length + idx
			}

			if idx < 0 || idx >= length {
				return Value{}, errorAt(site, "IndexError: buffer index %d out of bounds (length %d)", int(idxFloat), length)
			}

			return BoxNumber(float64(buf[idx])), nil
		}

		return Value{}, errorAt(site, "cannot index value of type '%s'", target.TypeOf())
	}
}

func (vm *Instance) emitArrayLiteral(node ast.ArrayLiteral) instruction {
	elements := make([]instruction, len(node.Elements))
	for i, el := range node.Elements {
		elements[i] = vm.compile(el)
	}

	return func(fbr *Fiber) (Value, Exception) {
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

	return func(fbr *Fiber) (Value, Exception) {
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

		return func(fbr *Fiber) (Value, Exception) {
			lhs, err := value(fbr)
			if err != nil {
				return lhs, err
			}

			if obj, ok := lhs.AsObject(); ok {
				_, exists := obj[fa.Rhs]
				return BoxBool(exists), nil
			}

			if obj, ok := lhs.AsUserStructInstance(); ok {
				return BoxBool(obj.HasField(index)), nil
			}

			// nothing else has fields
			return BoxBool(false), nil
		}
	}

	if sc, isSubscript := node.Value.(ast.Subscript); isSubscript {
		target := vm.compile(sc.Lhs)
		key := vm.compile(sc.Key)
		return func(fbr *Fiber) (Value, Exception) {
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
				if idxFloat, ok := k.AsFloat64(); ok {
					idx := int(idxFloat)
					length := len(arr.Data)
					if idx < 0 {
						idx = length + idx
					}
					return BoxBool(idx >= 0 && idx < length), nil
				}
				return BoxBool(false), nil
			}
			if si, ok := lhs.AsUserStructInstance(); ok {
				if keyStr, ok := k.AsString(); ok {
					return BoxBool(si.hasFieldByName(keyStr)), nil
				}
				return BoxBool(false), nil
			}
			return BoxBool(false), nil
		}
	}

	panic(fmt.Errorf("implement %T?", node.Value))
}

// compoundOperator returns the arithmetic behind a compound assignment like `+=`
func (vm *Instance) compoundOperator(node ast.MutableBinOp) func(fbr *Fiber, current, rhs Value) (Value, Exception) {
	site := vm.at(node)
	var apply func(Value, Value) (Value, bool)
	switch node.Operator {
	case ast.AddOp:
		apply = Value.Add
	case ast.SubOp:
		apply = Value.Sub
	case ast.MulOp:
		apply = Value.Mul
	case ast.DivOp:
		apply = Value.Div
	case ast.ModOp:
		apply = Value.Mod
	default:
		panic(vm.compileErrorAt(node, "unsupported compound operator '%v'", node.Operator))
	}

	symbol := node.Operator.String()
	return func(fbr *Fiber, current, rhs Value) (Value, Exception) {
		if result, ok := apply(current, rhs); ok {
			return result, nil
		}
		return Value{}, operatorError(site, symbol, current, rhs)
	}
}

// emitCompoundFieldAssign compiles `x.field op= value`, evaluating `x` exactly once
func (vm *Instance) emitCompoundFieldAssign(node ast.MutableBinOp, fa ast.FieldAccess) instruction {
	site := vm.at(node)
	target := vm.compile(fa.Lhs)
	rhs := vm.compile(node.Rhs)
	apply := vm.compoundOperator(node)
	index := fields.Get(fa.Rhs)

	return func(fbr *Fiber) (Value, Exception) {
		obj, exc := target(fbr)
		if exc != nil {
			return obj, exc
		}

		value, exc := rhs(fbr)
		if exc != nil {
			return value, exc
		}

		if usi, ok := obj.AsUserStructInstance(); ok {
			exists, exc := usi.UpdateField(index, func(current Value) (Value, Exception) {
				return apply(fbr, current, value)
			})
			if !exists {
				return Value{}, errorAt(site, "undefined field '%s' on %s", fa.Rhs, usi.InstanceOf.Name)
			}
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
			if !exists {
				return Value{}, errorAt(site, "Symbol '%s' not found in package '%s'.", fa.Rhs, pkg.name)
			}
			if field.IsStatic {
				return Value{}, errorAt(site, "Assignment to constant symbol '%v' of package '%v'.", fa.Rhs, pkg.name)
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

	return func(fbr *Fiber) (Value, Exception) {
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
			idxFloat, ok := keyVal.AsFloat64()
			if !ok {
				return Value{}, errorAt(site, "array index must be a number, got '%s'", keyVal.TypeOf())
			}
			idx := int(idxFloat)
			if float64(idx) != idxFloat {
				return Value{}, errorAt(site, "array index must be an integer, got %v", idxFloat)
			}
			length := len(arr.Data)
			if idx < 0 {
				idx = length + idx
			}
			if idx < 0 || idx >= length {
				return Value{}, errorAt(site, "IndexError: index %d out of bounds (length %d)", int(idxFloat), length)
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

		if usi, ok := obj.AsUserStructInstance(); ok {
			if !isString {
				return Value{}, errorAt(site, "struct field key must be a string, got '%s'", keyVal.TypeOf())
			}
			exists, exc := usi.updateFieldByName(keyStr, func(current Value) (Value, Exception) {
				return apply(fbr, current, value)
			})
			if !exists {
				return Value{}, errorAt(site, "undefined field '%s' on %s", keyStr, usi.InstanceOf.Name)
			}
			return Value{}, exc
		}

		return Value{}, errorAt(site, "cannot assign to subscript of type '%s'", obj.TypeOf())
	}
}
