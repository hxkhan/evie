package evie

import (
	"github.com/hk-32/evie/core"
	"github.com/hk-32/evie/core/std"
	"github.com/hk-32/evie/core/std/builtin"
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

func DefaultExports() map[string]any {
	std.Exports = map[string]any{}
	/* fs.Export()
	time.Export() */
	builtin.Export()
	return std.Exports
}
