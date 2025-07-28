package json

import (
	"encoding/json"

	"github.com/hxkhan/evie/vm"
)

func Constructor() map[string]*vm.Value {
	readFile := vm.BoxGoFunc(decode)

	return map[string]*vm.Value{
		"readFile": &readFile,
	}
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
