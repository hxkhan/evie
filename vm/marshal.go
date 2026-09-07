package vm

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/hxkhan/evie/vm/fields"
)

// BoxAny converts any Go value into an Evie Value.
func BoxAny(v any) (Value, error) {
	if v == nil {
		return Value{}, nil
	}

	switch val := v.(type) {
	case Value:
		return val, nil
	case *Value:
		if val == nil {
			return Value{}, nil
		}
		return *val, nil
	case bool:
		return BoxBool(val), nil
	case int:
		return BoxNumber(float64(val)), nil
	case int8:
		return BoxNumber(float64(val)), nil
	case int16:
		return BoxNumber(float64(val)), nil
	case int32:
		return BoxNumber(float64(val)), nil
	case int64:
		return BoxNumber(float64(val)), nil
	case uint:
		return BoxNumber(float64(val)), nil
	case uint8:
		return BoxNumber(float64(val)), nil
	case uint16:
		return BoxNumber(float64(val)), nil
	case uint32:
		return BoxNumber(float64(val)), nil
	case uint64:
		return BoxNumber(float64(val)), nil
	case float32:
		return BoxNumber(float64(val)), nil
	case float64:
		return BoxNumber(val), nil
	case string:
		return BoxString(val), nil
	case []byte:
		return BoxBuffer(val), nil
	case []Value:
		return BoxArray(NewArray(val...)), nil
	case *Array:
		return BoxArray(val), nil
	case Array:
		return BoxArray(&val), nil
	case map[fields.ID]Value:
		return BoxObject(val), nil
	case map[string]Value:
		obj := make(map[fields.ID]Value, len(val))
		for k, item := range val {
			obj[fields.Get(k)] = item
		}
		return BoxObject(obj), nil
	case map[string]any:
		obj := make(map[fields.ID]Value, len(val))
		for k, item := range val {
			boxed, err := BoxAny(item)
			if err != nil {
				return Value{}, err
			}
			obj[fields.Get(k)] = boxed
		}
		return BoxObject(obj), nil
	case []any:
		items := make([]Value, len(val))
		for i, item := range val {
			boxed, err := BoxAny(item)
			if err != nil {
				return Value{}, err
			}
			items[i] = boxed
		}
		return BoxArray(NewArray(items...)), nil
	case []string:
		items := make([]Value, len(val))
		for i, s := range val {
			items[i] = BoxString(s)
		}
		return BoxArray(NewArray(items...)), nil
	case []int:
		items := make([]Value, len(val))
		for i, n := range val {
			items[i] = BoxNumber(float64(n))
		}
		return BoxArray(NewArray(items...)), nil
	case []float64:
		items := make([]Value, len(val))
		for i, n := range val {
			items[i] = BoxNumber(n)
		}
		return BoxArray(NewArray(items...)), nil
	case *GoFunc:
		return BoxGoFunc(val), nil
	case GoFunc:
		return BoxGoFunc(&val), nil
	case GoFuncSignature:
		return BoxGoFunc(&GoFunc{Name: "native", Fn: val}), nil
	case *UserFn:
		return BoxUserFn(*val), nil
	case UserFn:
		return BoxUserFn(val), nil
	case *Task:
		return BoxTask(val), nil
	case CustomValue:
		return BoxCustom(val), nil
	case *Error:
		return BoxError(val), nil
	case error:
		return BoxError(CustomError("%s", val.Error())), nil
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer:
		if rv.IsNil() {
			return Value{}, nil
		}
		return BoxAny(rv.Elem().Interface())

	case reflect.Slice, reflect.Array:
		items := make([]Value, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			b, err := BoxAny(rv.Index(i).Interface())
			if err != nil {
				return Value{}, err
			}
			items[i] = b
		}
		return BoxArray(NewArray(items...)), nil

	case reflect.Map:
		obj := make(map[fields.ID]Value, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			keyStr := fmt.Sprint(iter.Key().Interface())
			valBoxed, err := BoxAny(iter.Value().Interface())
			if err != nil {
				return Value{}, err
			}
			obj[fields.Get(keyStr)] = valBoxed
		}
		return BoxObject(obj), nil

	case reflect.Struct:
		t := rv.Type()
		obj := make(map[fields.ID]Value, t.NumField())
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			name := f.Name
			if tag := f.Tag.Get("json"); tag != "" && tag != "-" {
				parts := strings.Split(tag, ",")
				if parts[0] != "" {
					name = parts[0]
				}
			}
			fieldVal := rv.Field(i).Interface()
			boxed, err := BoxAny(fieldVal)
			if err != nil {
				return Value{}, err
			}
			obj[fields.Get(name)] = boxed
		}
		return BoxObject(obj), nil

	case reflect.Func:
		return wrapGoFunction(rv)

	default:
		return Value{}, fmt.Errorf("cannot box type %T into Evie Value", v)
	}
}

func wrapGoFunction(fnVal reflect.Value) (Value, error) {
	fnType := fnVal.Type()
	numIn := fnType.NumIn()
	isVariadic := fnType.IsVariadic()

	gf := &GoFunc{
		Name:       "goFunc",
		Arguments:  numIn,
		IsVariadic: isVariadic,
		Fn: func(fbr *Fiber) (Value, Exception) {
			argCount := fbr.ArgCount()
			if isVariadic {
				if argCount < numIn-1 {
					return Value{}, CustomError("function requires at least %d argument(s), %d provided", numIn-1, argCount)
				}
			} else {
				if argCount != numIn {
					return Value{}, CustomError("function requires %d argument(s), %d provided", numIn, argCount)
				}
			}

			inArgs := make([]reflect.Value, numIn)
			fixedCount := numIn
			if isVariadic {
				fixedCount = numIn - 1
			}

			for i := 0; i < fixedCount; i++ {
				evieVal := fbr.GetLocal(int16(i))
				goVal, err := unmarshalToType(evieVal, fnType.In(i))
				if err != nil {
					return Value{}, CustomError("argument %d: %v", i, err)
				}
				if goVal == nil {
					inArgs[i] = reflect.Zero(fnType.In(i))
				} else {
					inArgs[i] = reflect.ValueOf(goVal)
				}
			}

			if isVariadic {
				elemType := fnType.In(numIn - 1).Elem()
				varCount := argCount - (numIn - 1)
				sliceVal := reflect.MakeSlice(fnType.In(numIn-1), varCount, varCount)
				for j := 0; j < varCount; j++ {
					evieVal := fbr.GetLocal(int16(numIn - 1 + j))
					goVal, err := unmarshalToType(evieVal, elemType)
					if err != nil {
						return Value{}, CustomError("variadic argument %d: %v", j, err)
					}
					if goVal == nil {
						sliceVal.Index(j).Set(reflect.Zero(elemType))
					} else {
						sliceVal.Index(j).Set(reflect.ValueOf(goVal))
					}
				}
				inArgs[numIn-1] = sliceVal
			}

			var out []reflect.Value
			if isVariadic {
				out = fnVal.CallSlice(inArgs)
			} else {
				out = fnVal.Call(inArgs)
			}

			if len(out) == 0 {
				return Value{}, nil
			}

			errType := reflect.TypeOf((*error)(nil)).Elem()
			if len(out) == 1 {
				if out[0].Type().Implements(errType) {
					if !out[0].IsNil() {
						err := out[0].Interface().(error)
						return Value{}, CustomError("%s", err.Error())
					}
					return Value{}, nil
				}
				res, err := BoxAny(out[0].Interface())
				if err != nil {
					return Value{}, CustomError("return conversion error: %v", err)
				}
				return res, nil
			}

			if len(out) == 2 && out[1].Type().Implements(errType) {
				if !out[1].IsNil() {
					err := out[1].Interface().(error)
					return Value{}, CustomError("%s", err.Error())
				}
				res, err := BoxAny(out[0].Interface())
				if err != nil {
					return Value{}, CustomError("return conversion error: %v", err)
				}
				return res, nil
			}

			// multiple return values: return array
			results := make([]Value, len(out))
			for i, o := range out {
				b, err := BoxAny(o.Interface())
				if err != nil {
					return Value{}, CustomError("return conversion error at %d: %v", i, err)
				}
				results[i] = b
			}
			return BoxArray(NewArray(results...)), nil
		},
	}

	return BoxGoFunc(gf), nil
}

func unmarshalToType(val Value, targetType reflect.Type) (any, error) {
	if targetType == reflect.TypeOf(Value{}) {
		return val, nil
	}

	if val.IsNil() {
		switch targetType.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Map, reflect.Interface:
			return nil, nil
		default:
			return nil, fmt.Errorf("cannot unmarshal nil into %v", targetType)
		}
	}

	if targetType.Kind() == reflect.Interface {
		return val.ToGo(), nil
	}

	switch targetType.Kind() {
	case reflect.Bool:
		if b, ok := val.AsBool(); ok {
			return b, nil
		}
		return val.IsTruthy(), nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if n, ok := val.AsFloat64(); ok {
			v := int64(n)
			switch targetType.Kind() {
			case reflect.Int:
				return int(v), nil
			case reflect.Int8:
				return int8(v), nil
			case reflect.Int16:
				return int16(v), nil
			case reflect.Int32:
				return int32(v), nil
			case reflect.Int64:
				return v, nil
			}
		}
		return nil, fmt.Errorf("expected number for %v, got %s", targetType, val.TypeOf())

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if n, ok := val.AsFloat64(); ok {
			v := uint64(n)
			switch targetType.Kind() {
			case reflect.Uint:
				return uint(v), nil
			case reflect.Uint8:
				return uint8(v), nil
			case reflect.Uint16:
				return uint16(v), nil
			case reflect.Uint32:
				return uint32(v), nil
			case reflect.Uint64:
				return v, nil
			}
		}
		return nil, fmt.Errorf("expected number for %v, got %s", targetType, val.TypeOf())

	case reflect.Float32:
		if n, ok := val.AsFloat64(); ok {
			return float32(n), nil
		}
		return nil, fmt.Errorf("expected number for %v, got %s", targetType, val.TypeOf())

	case reflect.Float64:
		if n, ok := val.AsFloat64(); ok {
			return n, nil
		}
		return nil, fmt.Errorf("expected number for %v, got %s", targetType, val.TypeOf())

	case reflect.String:
		if s, ok := val.AsString(); ok {
			return s, nil
		}
		return val.String(), nil

	case reflect.Slice:
		if targetType.Elem().Kind() == reflect.Uint8 {
			if buf, ok := val.AsBuffer(); ok {
				return buf, nil
			}
			if s, ok := val.AsString(); ok {
				return []byte(s), nil
			}
		}
		// general slice
		if arr, ok := val.AsArray(); ok {
			sliceVal := reflect.MakeSlice(targetType, len(arr.Data), len(arr.Data))
			elemType := targetType.Elem()
			for i, elem := range arr.Data {
				converted, err := unmarshalToType(elem, elemType)
				if err != nil {
					return nil, err
				}
				if converted == nil {
					sliceVal.Index(i).Set(reflect.Zero(elemType))
				} else {
					sliceVal.Index(i).Set(reflect.ValueOf(converted))
				}
			}
			return sliceVal.Interface(), nil
		}
		return nil, fmt.Errorf("expected array for %v, got %s", targetType, val.TypeOf())

	case reflect.Map:
		if obj, ok := val.AsObject(); ok {
			mapVal := reflect.MakeMapWithSize(targetType, len(obj))
			keyType := targetType.Key()
			valType := targetType.Elem()
			for k, v := range obj {
				keyName := fields.Lookup(k)
				kConverted, err := unmarshalToType(BoxString(keyName), keyType)
				if err != nil {
					return nil, err
				}
				vConverted, err := unmarshalToType(v, valType)
				if err != nil {
					return nil, err
				}
				var kVal, vVal reflect.Value
				if kConverted == nil {
					kVal = reflect.Zero(keyType)
				} else {
					kVal = reflect.ValueOf(kConverted)
				}
				if vConverted == nil {
					vVal = reflect.Zero(valType)
				} else {
					vVal = reflect.ValueOf(vConverted)
				}
				mapVal.SetMapIndex(kVal, vVal)
			}
			return mapVal.Interface(), nil
		}
		return nil, fmt.Errorf("expected object for %v, got %s", targetType, val.TypeOf())

	case reflect.Struct:
		target := reflect.New(targetType).Interface()
		if err := val.Unmarshal(target); err != nil {
			return nil, err
		}
		return reflect.ValueOf(target).Elem().Interface(), nil

	case reflect.Pointer:
		target := reflect.New(targetType.Elem()).Interface()
		if err := val.Unmarshal(target); err != nil {
			return nil, err
		}
		return target, nil

	default:
		return nil, fmt.Errorf("unsupported target type: %v", targetType)
	}
}

// ToGo converts an Evie Value into a standard Go type.
func (v Value) ToGo() any {
	if v.IsNil() {
		return nil
	}
	if b, ok := v.AsBool(); ok {
		return b
	}
	if n, ok := v.AsFloat64(); ok {
		return n
	}
	if s, ok := v.AsString(); ok {
		return s
	}
	if buf, ok := v.AsBuffer(); ok {
		return buf
	}
	if arr, ok := v.AsArray(); ok {
		res := make([]any, len(arr.Data))
		for i, elem := range arr.Data {
			res[i] = elem.ToGo()
		}
		return res
	}
	if obj, ok := v.AsObject(); ok {
		res := make(map[string]any, len(obj))
		for k, val := range obj {
			res[fields.Lookup(k)] = val.ToGo()
		}
		return res
	}
	if inst, ok := v.AsUserStructInstance(); ok {
		inst.MU.RLock()
		defer inst.MU.RUnlock()
		res := make(map[string]any, len(inst.Fields)+len(inst.Dynamic)+1)
		if inst.InstanceOf != nil {
			res["__struct"] = inst.InstanceOf.Name
			for i, fid := range inst.InstanceOf.Fields {
				if i < len(inst.Fields) {
					res[fields.Lookup(fid)] = inst.Fields[i].ToGo()
				}
			}
		}
		for f, val := range inst.Dynamic {
			res[fields.Lookup(f)] = val.ToGo()
		}
		return res
	}
	if err, ok := v.AsError(); ok {
		return err
	}
	if task, ok := v.AsTask(); ok {
		return task
	}
	if fn, ok := v.AsUserFn(); ok {
		return fn
	}
	if gf, ok := v.AsGoFunc(); ok {
		return gf
	}
	if cv, ok := v.AsCustom(); ok {
		return cv
	}
	if pkg, ok := v.AsPackage(); ok {
		return pkg
	}
	return v
}

// Interface is an alias for ToGo to follow Go standard conventions.
func (v Value) Interface() any {
	return v.ToGo()
}

// Unmarshal decodes the Evie Value into a Go target pointer.
func (v Value) Unmarshal(target any) error {
	data, err := json.Marshal(v.ToGo())
	if err != nil {
		return fmt.Errorf("evie unmarshal error: %w", err)
	}
	return json.Unmarshal(data, target)
}

// Int returns the integer representation of a numeric Value (0 if not a number).
func (v Value) Int() int {
	if n, ok := v.AsFloat64(); ok {
		return int(n)
	}
	return 0
}

// Int64 returns the int64 representation of a numeric Value (0 if not a number).
func (v Value) Int64() int64 {
	if n, ok := v.AsFloat64(); ok {
		return int64(n)
	}
	return 0
}

// Float64 returns the float64 representation of a numeric Value (0 if not a number).
func (v Value) Float64() float64 {
	if n, ok := v.AsFloat64(); ok {
		return n
	}
	return 0
}

// Bool returns the boolean representation of a Value (false if not a bool).
func (v Value) Bool() bool {
	if b, ok := v.AsBool(); ok {
		return b
	}
	return false
}
