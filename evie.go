package evie

import (
	"github.com/hk-32/evie/core"
	"github.com/hk-32/evie/core/std"
	"github.com/hk-32/evie/core/std/builtin"
	"github.com/hk-32/evie/core/std/fs"
	"github.com/hk-32/evie/core/std/time"
	"github.com/hk-32/evie/internal/ast"
	"github.com/hk-32/evie/internal/parser"
)

type Options struct {
	Optimise      bool // use specialised instructions
	PrintCode     bool // print the resulting byte-code
	TimeIt        bool // measure the execution time
	ObserveIt     bool // collect metrics (affects performance)
	TopLevelLogic bool // whether to only allow declarations at top level

	Exports map[string]core.Value // what should be made available to the user
}

var Defaults = Options{Optimise: true, Exports: DefaultExports()}
var cs *ast.CompilerState

func DefaultExports() map[string]core.Value {
	std.Exports = map[string]core.Value{}
	fs.Export()
	time.Export()
	builtin.Export()
	return std.Exports
}

func Setup(opts Options) {
	cs = ast.NewCompiler(opts.Optimise, opts.Exports)
}

func Reset() {

}

func FeedCode(input []byte) error {
	output, err := parser.Parse(input)
	if err != nil {
		return err
	}

	rt, err := cs.Compile(output)
	if err != nil {
		return err
	}

	err = rt.Initialize()
	if err != nil {
		return err
	}

	return nil
}

func GetGlobal(name string) *core.Value {
	return core.GetGlobal(name)
}

func WaitForNoActivity(name string) {
	core.WaitForNoActivity()
}
