package vm

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"unsafe"

	"github.com/hxkhan/evie/vm/fields"
)

// BoxAny converts any Go value into an Evie Value.
func BoxAny(v any) (Value, error) {
	return boxAny(v, nil)
}

func boxAny(v any, seen map[uintptr]bool) (Value, error) {
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
		if val == nil {
			return Value{}, nil
		}
		return BoxArray(val), nil
	case Array:
		return BoxArray(&val), nil
	case map[string]Value:
		return BoxObject(val), nil
	case map[string]any:
		if val == nil {
			return BoxObject(nil), nil
		}
		ptr := reflect.ValueOf(val).Pointer()
		if len(val) > 0 && ptr != 0 {
			if seen[ptr] {
				return Value{}, fmt.Errorf("cannot box cyclic map")
			}
			if seen == nil {
				seen = make(map[uintptr]bool)
			}
			seen[ptr] = true
			defer delete(seen, ptr)
		}
		obj := make(map[string]Value, len(val))
		for k, item := range val {
			boxed, err := boxAny(item, seen)
			if err != nil {
				return Value{}, err
			}
			obj[k] = boxed
		}
		return BoxObject(obj), nil
	case []any:
		ptr := reflect.ValueOf(val).Pointer()
		if len(val) > 0 && ptr != 0 {
			if seen[ptr] {
				return Value{}, fmt.Errorf("cannot box cyclic slice")
			}
			if seen == nil {
				seen = make(map[uintptr]bool)
			}
			seen[ptr] = true
			defer delete(seen, ptr)
		}
		items := make([]Value, len(val))
		for i, item := range val {
			boxed, err := boxAny(item, seen)
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
		if val == nil {
			return Value{}, nil
		}
		return BoxGoFunc(val), nil
	case GoFunc:
		return BoxGoFunc(&val), nil
	case NativeFn:
		if val == nil {
			return Value{}, nil
		}
		return BoxGoFunc(NewVariadic("native", 0, val)), nil
	case *userFn:
		if val == nil {
			return Value{}, nil
		}
		return boxUserFnPtr(val), nil
	case userFn:
		return boxUserFn(val), nil
	case *Task:
		if val == nil {
			return Value{}, nil
		}
		return boxTask(val), nil
	case *Error:
		if val == nil {
			return Value{}, nil
		}
		return boxError(val), nil
	case error:
		return boxError(customError("%s", val.Error())), nil
	}

	// a Go type the host registered travels as it is
	if boxed, ok := boxHost(v); ok {
		return boxed, nil
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer:
		if rv.IsNil() {
			return Value{}, nil
		}
		ptr := rv.Pointer()
		if ptr != 0 {
			if seen[ptr] {
				return Value{}, fmt.Errorf("cannot box cyclic pointer of type %T", v)
			}
			if seen == nil {
				seen = make(map[uintptr]bool)
			}
			seen[ptr] = true
			defer delete(seen, ptr)
		}
		return boxAny(rv.Elem().Interface(), seen)

	case reflect.Slice, reflect.Array:
		if rv.Kind() == reflect.Slice && rv.Len() > 0 {
			if ptr := rv.Pointer(); ptr != 0 {
				if seen[ptr] {
					return Value{}, fmt.Errorf("cannot box cyclic slice of type %T", v)
				}
				if seen == nil {
					seen = make(map[uintptr]bool)
				}
				seen[ptr] = true
				defer delete(seen, ptr)
			}
		}
		items := make([]Value, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			b, err := boxAny(rv.Index(i).Interface(), seen)
			if err != nil {
				return Value{}, err
			}
			items[i] = b
		}
		return BoxArray(NewArray(items...)), nil

	case reflect.Map:
		if rv.IsNil() {
			return BoxObject(nil), nil
		}
		if rv.Len() > 0 {
			if ptr := rv.Pointer(); ptr != 0 {
				if seen[ptr] {
					return Value{}, fmt.Errorf("cannot box cyclic map of type %T", v)
				}
				if seen == nil {
					seen = make(map[uintptr]bool)
				}
				seen[ptr] = true
				defer delete(seen, ptr)
			}
		}
		obj := make(map[string]Value, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			keyStr := fmt.Sprint(iter.Key().Interface())
			valBoxed, err := boxAny(iter.Value().Interface(), seen)
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
			if tag := f.Tag.Get("json"); tag != "" {
				if tag == "-" {
					continue
				}
				parts := strings.Split(tag, ",")
				if parts[0] != "" {
					name = parts[0]
				}
			}
			fieldVal := rv.Field(i).Interface()
			boxed, err := boxAny(fieldVal, seen)
			if err != nil {
				return Value{}, err
			}
			obj[name] = boxed
		}
		return BoxObject(obj), nil

	case reflect.Func:
		if rv.IsNil() {
			return Value{}, nil
		}
		return BoxGoFunc(Reflect(v)), nil

	default:
		return Value{}, fmt.Errorf("cannot box type %T into Evie Value", v)
	}
}

// isNilError safely checks whether a reflect.Value implementing error is nil
func isNilError(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Interface, reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
		return v.IsNil()
	}
	return false
}

// Reflect wraps any Go function as a native through reflection, converting
// each argument to the parameter type and boxing the results. It is the slow
// but universal path; the typed wrappers in wrap.go avoid reflection.
func Reflect(fn any) *GoFunc {
	if fn == nil {
		panic("Reflect: nil is not a function")
	}
	fnVal := reflect.ValueOf(fn)
	if !fnVal.IsValid() || fnVal.Kind() != reflect.Func {
		panic(fmt.Sprintf("Reflect: %T is not a function", fn))
	}
	if fnVal.IsNil() {
		panic(fmt.Sprintf("Reflect: nil function of type %T", fn))
	}
	fnType := fnVal.Type()
	numIn := fnType.NumIn()
	isVariadic := fnType.IsVariadic()
	fixedCount := numIn
	if isVariadic {
		fixedCount = numIn - 1
	}
	errType := reflect.TypeOf((*error)(nil)).Elem()

	gf := &GoFunc{Name: "native", arguments: fixedCount, isVariadic: isVariadic}
	gf.fn = func(c Call) (Value, error) {
		inArgs, err := marshalReflectArgs(c, fnType, numIn, fixedCount, isVariadic)
		if err != nil {
			return Value{}, err
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
		// if the last return value implements error, treat it as an error return
		if out[len(out)-1].Type().Implements(errType) {
			errVal := out[len(out)-1]
			if !isNilError(errVal) {
				return Value{}, errVal.Interface().(error)
			}
			out = out[:len(out)-1]
			if len(out) == 0 {
				return Value{}, nil
			}
		}
		if len(out) == 1 {
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

func marshalReflectArgs(c Call, fnType reflect.Type, numIn, fixedCount int, isVariadic bool) ([]reflect.Value, error) {
	if c.fbr.vm != nil && c.fbr.lock() {
		defer c.fbr.unlock()
	}
	inArgs := make([]reflect.Value, numIn)
	for i := 0; i < fixedCount; i++ {
		goVal, err := unmarshalToType(c.Arg(i), fnType.In(i))
		if err != nil {
			return nil, fmt.Errorf("argument %d: %v", i+1, err)
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
				return nil, fmt.Errorf("variadic argument %d: %v", j+1, err)
			}
			if goVal == nil {
				sliceVal.Index(j).Set(reflect.Zero(elemType))
			} else {
				sliceVal.Index(j).Set(reflect.ValueOf(goVal))
			}
		}
		inArgs[numIn-1] = sliceVal
	}
	return inArgs, nil
}

// convertNamed converts v to targetType when targetType is a defined/named type
// with the same underlying kind (e.g. time.Duration or type ID int64).
func convertNamed(v any, targetType reflect.Type) any {
	rv := reflect.ValueOf(v)
	if rv.Type() != targetType && rv.Type().ConvertibleTo(targetType) {
		return rv.Convert(targetType).Interface()
	}
	return v
}

func unmarshalToType(val Value, targetType reflect.Type) (any, error) {
	if targetType == reflect.TypeOf(Value{}) {
		return val, nil
	}

	if hv, _, ok := val.AsHost(); ok {
		if hv == nil {
			switch targetType.Kind() {
			case reflect.Pointer, reflect.Slice, reflect.Map, reflect.Interface:
				return nil, nil
			default:
				return nil, fmt.Errorf("cannot pass nil %s as %v", val.TypeOf(), targetType)
			}
		}
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
		goVal := val.ToGo()
		if targetType.NumMethod() == 0 {
			return goVal, nil
		}
		if goVal != nil && reflect.TypeOf(goVal).AssignableTo(targetType) {
			return goVal, nil
		}
		return nil, fmt.Errorf("cannot pass %s as %v", val.TypeOf(), targetType)
	}

	switch targetType.Kind() {
	case reflect.Bool:
		var b bool
		if bv, ok := val.AsBool(); ok {
			b = bv
		} else {
			b = val.IsTruthy()
		}
		return convertNamed(b, targetType), nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, ok := val.AsFloat64()
		if !ok {
			return nil, fmt.Errorf("expected number for %v, got %s", targetType, val.TypeOf())
		}
		if math.IsNaN(n) || math.IsInf(n, 0) || n != math.Trunc(n) {
			return nil, fmt.Errorf("expected integer for %v, got %v", targetType, n)
		}
		var minV, maxExcl float64
		switch targetType.Kind() {
		case reflect.Int:
			minV, maxExcl = float64(math.MinInt), -float64(math.MinInt)
		case reflect.Int8:
			minV, maxExcl = math.MinInt8, math.MaxInt8+1
		case reflect.Int16:
			minV, maxExcl = math.MinInt16, math.MaxInt16+1
		case reflect.Int32:
			minV, maxExcl = math.MinInt32, math.MaxInt32+1
		case reflect.Int64:
			minV, maxExcl = float64(math.MinInt64), -float64(math.MinInt64)
		}
		if n < minV || n >= maxExcl {
			return nil, fmt.Errorf("value %v out of range for %v", n, targetType)
		}
		v := int64(n)
		switch targetType.Kind() {
		case reflect.Int:
			return convertNamed(int(v), targetType), nil
		case reflect.Int8:
			return convertNamed(int8(v), targetType), nil
		case reflect.Int16:
			return convertNamed(int16(v), targetType), nil
		case reflect.Int32:
			return convertNamed(int32(v), targetType), nil
		default:
			return convertNamed(v, targetType), nil
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, ok := val.AsFloat64()
		if !ok {
			return nil, fmt.Errorf("expected number for %v, got %s", targetType, val.TypeOf())
		}
		if math.IsNaN(n) || math.IsInf(n, 0) || n != math.Trunc(n) || n < 0 {
			return nil, fmt.Errorf("expected non-negative integer for %v, got %v", targetType, n)
		}
		var maxExcl float64
		switch targetType.Kind() {
		case reflect.Uint:
			maxExcl = float64(math.MaxUint) + 1
		case reflect.Uint8:
			maxExcl = math.MaxUint8 + 1
		case reflect.Uint16:
			maxExcl = math.MaxUint16 + 1
		case reflect.Uint32:
			maxExcl = math.MaxUint32 + 1
		case reflect.Uint64:
			maxExcl = float64(math.MaxUint64)
		}
		if n >= maxExcl {
			return nil, fmt.Errorf("value %v out of range for %v", n, targetType)
		}
		v := uint64(n)
		switch targetType.Kind() {
		case reflect.Uint:
			return convertNamed(uint(v), targetType), nil
		case reflect.Uint8:
			return convertNamed(uint8(v), targetType), nil
		case reflect.Uint16:
			return convertNamed(uint16(v), targetType), nil
		case reflect.Uint32:
			return convertNamed(uint32(v), targetType), nil
		default:
			return convertNamed(v, targetType), nil
		}

	case reflect.Float32:
		if n, ok := val.AsFloat64(); ok {
			return convertNamed(float32(n), targetType), nil
		}
		return nil, fmt.Errorf("expected number for %v, got %s", targetType, val.TypeOf())

	case reflect.Float64:
		if n, ok := val.AsFloat64(); ok {
			return convertNamed(n, targetType), nil
		}
		return nil, fmt.Errorf("expected number for %v, got %s", targetType, val.TypeOf())

	case reflect.String:
		if s, ok := val.AsString(); ok {
			return convertNamed(s, targetType), nil
		}
		return convertNamed(val.String(), targetType), nil

	case reflect.Slice:
		if targetType.Elem().Kind() == reflect.Uint8 {
			if buf, ok := val.AsBuffer(); ok {
				return convertNamed(buf, targetType), nil
			}
			if s, ok := val.AsString(); ok {
				return convertNamed([]byte(s), targetType), nil
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
	return v.toGo(nil)
}

func (v Value) toGo(seen map[unsafe.Pointer]any) any {
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
		ptr := unsafe.Pointer(arr)
		if existing, ok := seen[ptr]; ok {
			return existing
		}
		if seen == nil {
			seen = make(map[unsafe.Pointer]any)
		}
		res := make([]any, len(arr.Data))
		seen[ptr] = res
		defer delete(seen, ptr)
		for i, elem := range arr.Data {
			res[i] = elem.toGo(seen)
		}
		return res
	}
	if obj, ok := v.AsObject(); ok {
		ptr := *(*unsafe.Pointer)(v.pointer)
		if ptr != nil {
			if existing, ok := seen[ptr]; ok {
				return existing
			}
		}
		if seen == nil {
			seen = make(map[unsafe.Pointer]any)
		}
		res := make(map[string]any, len(obj))
		if ptr != nil {
			seen[ptr] = res
			defer delete(seen, ptr)
		}
		for k, val := range obj {
			res[k] = val.toGo(seen)
		}
		return res
	}
	if inst, ok := v.asStructInstance(); ok {
		ptr := unsafe.Pointer(inst)
		if existing, ok := seen[ptr]; ok {
			return existing
		}
		if seen == nil {
			seen = make(map[unsafe.Pointer]any)
		}
		res := make(map[string]any, len(inst.Fields)+len(inst.Dynamic)+1)
		seen[ptr] = res
		defer delete(seen, ptr)
		if inst.InstanceOf != nil {
			res["__struct"] = inst.InstanceOf.Name
			for i, fid := range inst.InstanceOf.Fields {
				if i < len(inst.Fields) {
					res[fields.Lookup(fid)] = inst.Fields[i].toGo(seen)
				}
			}
		}
		for f, val := range inst.Dynamic {
			res[f] = val.toGo(seen)
		}
		return res
	}
	if err, ok := v.AsError(); ok {
		return err
	}
	if task, ok := v.AsTask(); ok {
		return task
	}
	if _, ok := v.asUserFn(); ok {
		return v
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

// MarshalJSON implements json.Marshaler for Evie values. It encodes nil, bool,
// finite numbers, strings, arrays, objects and struct instances (without
// synthetic metadata keys) and rejects cycles, NaN/Inf, and non-JSON values
// such as functions, tasks, packages, errors and buffers.
func (v Value) MarshalJSON() ([]byte, error) {
	var b strings.Builder
	if err := v.writeJSON(&b, nil); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

func (v Value) writeJSON(b *strings.Builder, seen map[unsafe.Pointer]bool) error {
	switch v.pointer {
	case nil:
		b.WriteString("null")
		return nil
	case boolType:
		if v.scalar == 0 {
			b.WriteString("false")
		} else {
			b.WriteString("true")
		}
		return nil
	case f64Type:
		f := math.Float64frombits(v.scalar)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return fmt.Errorf("cannot encode non-finite number (%v) as JSON", f)
		}
		data, err := json.Marshal(f)
		if err != nil {
			return err
		}
		b.Write(data)
		return nil
	}

	switch v.scalar {
	case kindString:
		data, err := json.Marshal(*(*string)(v.pointer))
		if err != nil {
			return err
		}
		b.Write(data)
		return nil

	case kindArray:
		arr := (*Array)(v.pointer)
		ptr := unsafe.Pointer(arr)
		if seen[ptr] {
			return fmt.Errorf("cannot encode cyclic array as JSON")
		}
		if seen == nil {
			seen = make(map[unsafe.Pointer]bool)
		}
		seen[ptr] = true
		defer delete(seen, ptr)

		b.WriteByte('[')
		for i, elem := range arr.Data {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := elem.writeJSON(b, seen); err != nil {
				return err
			}
		}
		b.WriteByte(']')
		return nil

	case kindObject:
		obj := *(*map[string]Value)(v.pointer)
		ptr := *(*unsafe.Pointer)(v.pointer)
		if ptr != nil {
			if seen[ptr] {
				return fmt.Errorf("cannot encode cyclic object as JSON")
			}
			if seen == nil {
				seen = make(map[unsafe.Pointer]bool)
			}
			seen[ptr] = true
			defer delete(seen, ptr)
		}
		b.WriteByte('{')
		for i, k := range sortedKeys(obj) {
			if i > 0 {
				b.WriteByte(',')
			}
			keyJSON, err := json.Marshal(k)
			if err != nil {
				return err
			}
			b.Write(keyJSON)
			b.WriteByte(':')
			if err := obj[k].writeJSON(b, seen); err != nil {
				return err
			}
		}
		b.WriteByte('}')
		return nil

	case kindStructInstance:
		inst := (*structInstance)(v.pointer)
		ptr := unsafe.Pointer(inst)
		if seen[ptr] {
			return fmt.Errorf("cannot encode cyclic struct '%s' as JSON", inst.InstanceOf.Name)
		}
		if seen == nil {
			seen = make(map[unsafe.Pointer]bool)
		}
		seen[ptr] = true
		defer delete(seen, ptr)

		b.WriteByte('{')
		first := true
		writeEntry := func(name string, val Value) error {
			if !first {
				b.WriteByte(',')
			}
			first = false
			keyJSON, err := json.Marshal(name)
			if err != nil {
				return err
			}
			b.Write(keyJSON)
			b.WriteByte(':')
			return val.writeJSON(b, seen)
		}
		if inst.InstanceOf != nil {
			for i, fid := range inst.InstanceOf.Fields {
				if i < len(inst.Fields) {
					if err := writeEntry(fields.Lookup(fid), inst.Fields[i]); err != nil {
						return err
					}
				}
			}
		}
		for _, name := range sortedKeys(inst.Dynamic) {
			if err := writeEntry(name, inst.Dynamic[name]); err != nil {
				return err
			}
		}
		b.WriteByte('}')
		return nil

	case kindHost:
		if m, ok := v.host().val.(json.Marshaler); ok {
			data, err := m.MarshalJSON()
			if err != nil {
				return err
			}
			b.Write(data)
			return nil
		}
	}

	return fmt.Errorf("cannot encode value of type %s as JSON", v.TypeOf())
}

// Unmarshal decodes the Evie Value into a Go target pointer.
func (v Value) Unmarshal(target any) error {
	data, err := json.Marshal(v.ToGo())
	if err != nil {
		return fmt.Errorf("evie unmarshal error: %w", err)
	}
	return json.Unmarshal(data, target)
}
