package fs

import (
	"os"

	"github.com/hk-32/evie/core"
	"github.com/hk-32/evie/std"
)

func Export() {
	std.ImportFn(readFile)
}

func readFile(fileName core.Value) (core.Value, error) {
	if fileName, ok := fileName.AsString(); ok {
		return core.NewTask(func() (core.Value, error) {
			bytes, err := os.ReadFile(fileName)
			return core.BoxBuffer(bytes), err
		}), nil
	}
	return core.Value{}, core.ErrTypes
}
