package std

import (
	"reflect"
	"runtime"
	"strings"

	"github.com/hxkhan/evie/vm"
)

var Exports map[string]vm.Value

func ImportFn[T vm.GoFunc](callable T) {
	// get name of the function
	path := runtime.FuncForPC(reflect.ValueOf(callable).Pointer()).Name()
	parts := strings.Split(path, "/")
	fullname := parts[len(parts)-1]

	v := vm.BoxGoFunc(callable)

	if name, found := strings.CutPrefix(fullname, "builtin."); found {
		Exports[name] = v
		return
	}

	Exports[fullname] = v
}

func ImportOther(name string, v vm.Value) {
	Exports[name] = v
}
