package vm

import (
	"reflect"
	rt "runtime"
	"strings"
)

func packageName(fn PackageContructor) string {
	// get name of the function
	path := rt.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
	parts := strings.Split(path, "/")
	fullname := parts[len(parts)-1]
	parts = strings.Split(fullname, ".")

	if len(parts) != 2 {
		panic("expected 2 parts in constructor path")
	}

	return parts[0]
}
