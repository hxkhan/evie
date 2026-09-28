package json

import (
	"encoding/json"

	"github.com/hxkhan/evie/vm"
)

// Construct builds the json package
func Construct() *vm.Package {
	pkg := vm.NewPackage("json")
	pkg.Func("decode", 1, func(c vm.Call) (vm.Value, error) {
		str, err := c.String(0)
		if err != nil {
			return vm.Value{}, err
		}
		var decoded any
		if err := json.Unmarshal([]byte(str), &decoded); err != nil {
			return vm.Value{}, err
		}
		return vm.BoxAny(decoded)
	})
	pkg.Func("encode", 1, func(c vm.Call) (vm.Value, error) {
		data, err := json.Marshal(c.Arg(0).ToGo())
		if err != nil {
			return vm.Value{}, err
		}
		return vm.BoxString(string(data)), nil
	})
	return pkg
}
