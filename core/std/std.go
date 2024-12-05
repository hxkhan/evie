package std

import (
	"reflect"
	"runtime"
	"strings"

	"github.com/hk-32/evie/core"
)

var Exports map[string]core.Value

func ImportFn[T core.ValidFuncTypes](callable T) {
	// get name of the function
	path := runtime.FuncForPC(reflect.ValueOf(callable).Pointer()).Name()
	parts := strings.Split(path, "/")
	fullname := parts[len(parts)-1]

	v := core.BoxFunc(callable)

	if name, found := strings.CutPrefix(fullname, "builtin."); found {
		Exports[name] = v
		return
	}

	Exports[fullname] = v
}

func ImportOther(name string, v core.Value) {
	Exports[name] = v
}
