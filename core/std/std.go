package std

import (
	"strings"

	"github.com/hk-32/evie/core"
)

var Exports map[string]any

func ImportFn[T core.ValidFnTypes](callable T) {
	fn := core.NativeFn[T]{Callable: callable}
	name := fn.Name()

	if name, found := strings.CutPrefix(name, "builtin."); found {
		Exports[name] = fn
		return
	}

	Exports[name] = fn
}

func ImportOther(name string, value any) {
	Exports[name] = value
}
