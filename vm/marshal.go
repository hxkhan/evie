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
	case map[string]Value:
		return BoxObject(val), nil
	case map[string]any:
		obj := make(map[string]Value, len(val))
		for k, item := range val {
			boxed, err := BoxAny(item)
			if err != nil {
				return Value{}, err
			}
			obj[k] = boxed
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
	case NativeFn:
		return BoxGoFunc(NewVariadic("native", 0, val)), nil
	case *UserFn:
		return BoxUserFn(*val), nil
	case UserFn:
		return BoxUserFn(val), nil
	case *Task:
		return BoxTask(val), nil
	case *Error:
		return BoxError(val), nil
	case error:
		return BoxError(CustomError("%s", val.Error())), nil
	}

	// a Go type the host registered travels as it is
	if boxed, ok := BoxHost(v); ok {
		return boxed, nil
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
		obj := make(map[string]Value, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			keyStr := fmt.Sprint(iter.Key().Interface())
			valBoxed, err := BoxAny(iter.Value().Interface())
			if err != nil {
				return Value{}, err
			}
			obj[keyStr] = valBoxed
		}
		return BoxObject(obj), nil

	case reflect.Struct:
		t := rv.Type()
		obj := make(map[string]Value, t.NumField())
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
			obj[name] = boxed
		}
		return BoxObject(obj), nil

	case reflect.Func:
		return BoxGoFunc(Reflect(v)), nil

	default:
		return Value{}, fmt.Errorf("cannot box type %T into Evie Value", v)
	}
}

// Reflect wraps any Go function as a native through reflection, converting
// each argument to the parameter type and boxing the results. It is the slow
// but universal path; the typed wrappers in wrap.go avoid reflection.
func Reflect(fn any) *GoFunc {
	fnVal := reflect.ValueOf(fn)
	fnType := fnVal.Type()
	if fnType.Kind() != reflect.Func {
		panic(fmt.Sprintf("Reflect: %T is not a function", fn))
	}
	numIn := fnType.NumIn()
	isVariadic := fnType.IsVariadic()
	fixedCount := numIn
	if isVariadic {
		fixedCount = numIn - 1
	}
	errType := reflect.TypeOf((*error)(nil)).Elem()

	gf := &GoFunc{Name: "native", Arguments: fixedCount, IsVariadic: isVariadic}
	gf.Fn = func(c Call) (Value, error) {
		inArgs := make([]reflect.Value, numIn)
		for i := 0; i < fixedCount; i++ {
			goVal, err := unmarshalToType(c.Arg(i), fnType.In(i))
			if err != nil {
				return Value{}, fmt.Errorf("argument %d: %v", i+1, err)
			}
			if goVal == nil {
				inArgs[i] = reflect.Zero(fnType.In(i))
			} else {
				inArgs[i] = reflect.ValueOf(goVal)
			}
		}

		if isVariadic {
			elemType := fnType.In(numIn - 1).Elem()
			varCount := c.Len() - fixedCount
			sliceVal := reflect.MakeSlice(fnType.In(numIn-1), varCount, varCount)
			for j := 0; j < varCount; j++ {
				goVal, err := unmarshalToType(c.Arg(fixedCount+j), elemType)
				if err != nil {
					return Value{}, fmt.Errorf("variadic argument %d: %v", j+1, err)
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

		switch {
		case len(out) == 0:
			return Value{}, nil
		case len(out) == 1 && out[0].Type().Implements(errType):
			if !out[0].IsNil() {
				return Value{}, out[0].Interface().(error)
			}
			return Value{}, nil
		case len(out) == 1:
			return BoxAny(out[0].Interface())
		case len(out) == 2 && out[1].Type().Implements(errType):
			if !out[1].IsNil() {
				return Value{}, out[1].Interface().(error)
			}
			return BoxAny(out[0].Interface())
		}

		// multiple return values become an array
		results := make([]Value, len(out))
		for i, o := range out {
			b, err := BoxAny(o.Interface())
			if err != nil {
				return Value{}, fmt.Errorf("return value %d: %v", i+1, err)
			}
			results[i] = b
		}
		return BoxArray(NewArray(results...)), nil
	}
	return gf
}

func unmarshalToType(val Value, targetType reflect.Type) (any, error) {
	if targetType == reflect.TypeOf(Value{}) {
		return val, nil
	}

	if hv, _, ok := val.AsHost(); ok {
		if reflect.TypeOf(hv).AssignableTo(targetType) {
			return hv, nil
		}
		return nil, fmt.Errorf("cannot pass %s as %v", val.TypeOf(), targetType)
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
				kConverted, err := unmarshalToType(BoxString(k), keyType)
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
			res[k] = val.ToGo()
		}
		return res
	}
	if inst, ok := v.AsUserStructInstance(); ok {
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
			res[f] = val.ToGo()
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
	if hv, _, ok := v.AsHost(); ok {
		return hv
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
