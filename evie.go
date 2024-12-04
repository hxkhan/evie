package evie

import (
	"github.com/hk-32/evie/core"
	"github.com/hk-32/evie/core/std"
	"github.com/hk-32/evie/core/std/builtin"
	"github.com/hk-32/evie/core/std/fs"
	"github.com/hk-32/evie/core/std/time"
)

type Program interface {
	Start() (core.Value, error)
	PrintCode()
}

/* func NewProgramFromAST(p ast.Package, optimise bool, observe bool) (Program, error) {
	std.Exports = map[string]any{}

	fs.Export()
	time.Export()
	builtin.Export()

	return p.Compile(optimise, std.Exports)
} */

func DefaultExports() map[string]core.Value {
	std.Exports = map[string]core.Value{}
	fs.Export()
	time.Export()
	builtin.Export()
	return std.Exports
}
