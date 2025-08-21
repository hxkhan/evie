package json

import (
	"encoding/json"

	"hxkhan.dev/evie/vm"
)

func Construct() vm.Package {
	pkg := vm.NewHostPackage("json")
	pkg.SetSymbol("decode", vm.BoxGoFunc(decode))
	return pkg
}

func decode(v vm.Value) (vm.Value, *vm.Exception) {
	str, ok := v.AsString()
	if ok {
		var v any
		err := json.Unmarshal([]byte(str), &v)
		if err != nil {
			return vm.Value{}, vm.CustomError(err.Error())
		}

		switch v.(type) {
		case []any:

		}

		panic("implement")

		//return v, err
	}

	return vm.Value{}, vm.ErrTypes
}
