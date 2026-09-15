package vm

import (
	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/vm/fields"
)

func (vm *Instance) emitIdentDec(node ast.Decl) instruction {
	index, success := vm.cp.closures.Last(0).scope.Declare(node.Name, node.IsStatic)
	if !success {
		panic(CompileErrorf("double declaration of %s", node.Name))
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
		panic(err)
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

	panic("ayo what")
}

func (vm *Instance) emitAssign(node ast.Assign) instruction {
	// handle variables
	if iGet, isIdentGet := node.Lhs.(ast.Ident); isIdentGet {
		variable, err := vm.cp.reach(iGet.Name)
		if err != nil {
			panic(err)
		}

		// compile new value
		value := vm.compile(node.Value)

		switch v := variable.(type) {
		case local:
			if v.isStatic {
				panic(CompileErrorf("Assignment to constant binding '%v'.", iGet.Name))
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
				panic(CompileErrorf("Assignment to constant binding '%v'.", iGet.Name))
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
							usi.MU.Lock()
							if usi.InstanceOf == ustruct && slot < len(usi.Fields) {
								usi.Fields[slot] = v
								usi.MU.Unlock()
								return Value{}, nil
							}
							usi.MU.Unlock()
							usi.SetField(index, v)
							return Value{}, nil
						}
						return Value{}, RuntimeExceptionF("cannot assign field '%s' on %s", fa.Rhs, thisVal.TypeOf())
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

					obj[index] = value
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

				panic("implement type")
			}

		case Global:
			if lhs.IsStatic {
				if pkg, ok := lhs.asPackage(); ok {
					field, exists := pkg.globals[fields.Get(fa.Rhs)]
					if !exists {
						panic(TypeErrorF("Symbol '%s' not found in package '%s'.", fa.Rhs, pkg.name))
					}

					if field.IsStatic {
						panic(TypeErrorF("Assignment to constant symbol '%v' of package '%v'.", fa.Rhs, pkg.name))
					}

					// compile new value & return setter
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

				panic("implement type")
			}

			// compile new value & return setter
			value := vm.compile(node.Value)
			index := fields.Get(fa.Rhs)
			return func(fbr *Fiber) (Value, Exception) {
				if pkg, ok := lhs.asPackage(); ok {
					field, exists := pkg.globals[index]
					if !exists {
						return Value{}, TypeErrorF("Symbol '%s' not found in package '%s'.", fa.Rhs, pkg.name)
					}

					if field.IsStatic {
						return Value{}, TypeErrorF("Assignment to constant symbol '%v' of package '%v'.", fa.Rhs, pkg.name)
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

					obj[index] = value
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

				panic("implement type")
			}
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
					return Value{}, TypeErrorF("array index must be a number, got '%s'", keyVal.TypeOf())
				}
				idx := int(idxFloat)
				if float64(idx) != idxFloat {
					return Value{}, RuntimeExceptionF("array index must be an integer, got %v", idxFloat)
				}

				arr.MU.Lock()
				defer arr.MU.Unlock()

				length := len(arr.Data)
				if idx < 0 {
					idx = length + idx
				}

				if idx < 0 || idx >= length {
					return Value{}, RuntimeExceptionF("IndexError: index %d out of bounds (length %d)", int(idxFloat), length)
				}

				arr.Data[idx] = newVal
				return Value{}, nil
			}

			// 2. Object key assignment: obj[key] = newVal
			if obj, ok := target.AsObject(); ok {
				keyStr, ok := keyVal.AsString()
				if !ok {
					return Value{}, TypeErrorF("object key must be a string, got '%s'", keyVal.TypeOf())
				}

				obj[fields.Get(keyStr)] = newVal
				return Value{}, nil
			}

			// 3. Struct instance field assignment: structInst[key] = newVal
			if si, ok := target.AsUserStructInstance(); ok {
				keyStr, ok := keyVal.AsString()
				if !ok {
					return Value{}, TypeErrorF("struct field key must be a string, got '%s'", keyVal.TypeOf())
				}

				si.SetField(fields.Get(keyStr), newVal)
				return Value{}, nil
			}

			return Value{}, TypeErrorF("cannot assign to subscript of type '%s'", target.TypeOf())
		}
	}

	panic("ayo what")
}

func (vm *Instance) emitFieldAccess(node ast.FieldAccess) instruction {
	index := fields.Get(node.Rhs)

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
							usi.MU.RLock()
							if usi.InstanceOf == ustruct && slot < len(usi.Fields) {
								val := usi.Fields[slot]
								usi.MU.RUnlock()
								return val, nil
							}
							usi.MU.RUnlock()
							if val, exists := usi.GetField(index); exists {
								return val, nil
							}
						}
						if field, exists := thisVal.getField(index); exists {
							return field, nil
						}
						return Value{}, RuntimeExceptionF("undefined symbol '%v' in '%v'", node.Rhs, node)
					}
				}
			}

			var cachedStruct *UserStruct
			var cachedSlot int = -1
			return func(fbr *Fiber) (Value, Exception) {
				v := fbr.get(lhs)
				if v.scalar == kindStructInstance && !isKnown(v.pointer) {
					usi := (*UserStructInstance)(v.pointer)
					usi.MU.RLock()
					if usi.InstanceOf == cachedStruct && cachedSlot >= 0 && cachedSlot < len(usi.Fields) {
						val := usi.Fields[cachedSlot]
						usi.MU.RUnlock()
						return val, nil
					}
					usi.MU.RUnlock()

					if slot, ok := usi.InstanceOf.FieldOffsets[index]; ok {
						cachedStruct = usi.InstanceOf
						cachedSlot = slot
						usi.MU.RLock()
						val := usi.Fields[slot]
						usi.MU.RUnlock()
						return val, nil
					}
				}
				if field, exists := v.getField(index); exists {
					return field, nil
				}
				return Value{}, RuntimeExceptionF("undefined symbol '%v' in '%v'", node.Rhs, node)
			}

		case Value:
			// I don't think this gets used tbh because using vm.evaluate(ast.FieldAccess) bypasses this
			if field, exists := lhs.getField(index); exists {
				return func(fbr *Fiber) (Value, Exception) {
					return field, nil
				}
			}
			panic(TypeErrorF("undefined symbol '%v' in '%v'", node.Rhs, node))

		case Global:
			// global non-static binding
			return func(fbr *Fiber) (Value, Exception) {
				if field, exists := lhs.getField(index); exists {
					return field, nil
				}
				panic(TypeErrorF("undefined symbol '%v' in '%v'", node.Rhs, node))
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
			usi.MU.RLock()
			if usi.InstanceOf == cachedStruct && cachedSlot >= 0 && cachedSlot < len(usi.Fields) {
				val := usi.Fields[cachedSlot]
				usi.MU.RUnlock()
				return val, nil
			}
			usi.MU.RUnlock()

			if slot, ok := usi.InstanceOf.FieldOffsets[index]; ok {
				cachedStruct = usi.InstanceOf
				cachedSlot = slot
				usi.MU.RLock()
				val := usi.Fields[slot]
				usi.MU.RUnlock()
				return val, nil
			}
		}

		if field, exists := lhs.getField(index); exists {
			return field, nil
		}

		return Value{}, RuntimeExceptionF("undefined symbol '%v' in '%v'", node.Rhs, node)
	}
}

func (vm *Instance) emitSubscript(node ast.Subscript) instruction {
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
				return Value{}, TypeErrorF("array index must be a number, got '%s'", indexVal.TypeOf())
			}
			idx := int(idxFloat)
			if float64(idx) != idxFloat {
				return Value{}, RuntimeExceptionF("array index must be an integer, got %v", idxFloat)
			}

			arr.MU.RLock()
			defer arr.MU.RUnlock()

			length := len(arr.Data)
			if idx < 0 {
				idx = length + idx
			}

			if idx < 0 || idx >= length {
				return Value{}, RuntimeExceptionF("IndexError: index %d out of bounds (length %d)", int(idxFloat), length)
			}

			return arr.Data[idx], nil
		}

		// 2. Object key access: obj["key"]
		if obj, ok := target.AsObject(); ok {
			keyStr, ok := indexVal.AsString()
			if !ok {
				return Value{}, TypeErrorF("object key must be a string, got '%s'", indexVal.TypeOf())
			}

			val, exists := obj[fields.Get(keyStr)]
			if !exists {
				return Value{}, nil
			}
			return val, nil
		}

		// 3. Struct instance field access: structInstance["field"]
		if si, ok := target.AsUserStructInstance(); ok {
			keyStr, ok := indexVal.AsString()
			if !ok {
				return Value{}, TypeErrorF("struct field key must be a string, got '%s'", indexVal.TypeOf())
			}

			val, exists := si.GetField(fields.Get(keyStr))
			if !exists {
				return Value{}, RuntimeExceptionF("undefined field '%s' in struct", keyStr)
			}
			return val, nil
		}

		// 4. String indexing: "hello"[1] -> "e"
		if str, ok := target.AsString(); ok {
			idxFloat, ok := indexVal.AsFloat64()
			if !ok {
				return Value{}, TypeErrorF("string index must be a number, got '%s'", indexVal.TypeOf())
			}
			idx := int(idxFloat)
			if float64(idx) != idxFloat {
				return Value{}, RuntimeExceptionF("string index must be an integer, got %v", idxFloat)
			}

			length := len(str)
			if idx < 0 {
				idx = length + idx
			}

			if idx < 0 || idx >= length {
				return Value{}, RuntimeExceptionF("IndexError: string index %d out of bounds (length %d)", int(idxFloat), length)
			}

			return BoxString(string(str[idx])), nil
		}

		// 5. Buffer indexing: buf[0] -> byte value as number
		if buf, ok := target.AsBuffer(); ok {
			idxFloat, ok := indexVal.AsFloat64()
			if !ok {
				return Value{}, TypeErrorF("buffer index must be a number, got '%s'", indexVal.TypeOf())
			}
			idx := int(idxFloat)
			if float64(idx) != idxFloat {
				return Value{}, RuntimeExceptionF("buffer index must be an integer, got %v", idxFloat)
			}

			length := len(buf)
			if idx < 0 {
				idx = length + idx
			}

			if idx < 0 || idx >= length {
				return Value{}, RuntimeExceptionF("IndexError: buffer index %d out of bounds (length %d)", int(idxFloat), length)
			}

			return BoxNumber(float64(buf[idx])), nil
		}

		return Value{}, TypeErrorF("cannot index value of type '%s'", target.TypeOf())
	}
}
