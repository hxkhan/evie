package json

import (
	json "encoding/json/v2"

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
		var val vm.Value
		if err := json.Unmarshal([]byte(str), &val); err != nil {
			return vm.Value{}, err
		}
		return val, nil
	})
	pkg.Func("encode", 1, func(c vm.Call) (vm.Value, error) {
		data, err := c.Arg(0).MarshalJSON()
		if err != nil {
			return vm.Value{}, err
		}
		return vm.BoxString(string(data)), nil
	})
	// what tools such as editors show for each member
	pkg.Document("encode", "encode(value)", "The value as JSON text; rejects NaN, infinities, functions, tasks, errors and cycles.")
	pkg.Document("decode", "decode(text)", "The value that JSON text describes.")
	return pkg
}
