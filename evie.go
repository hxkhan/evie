// contains package level state which means you cannot have multiple instance of evie running at the same time,
// this was done for performance reasons and for a lack of a better way to make concurrency work with struct level state
package evie

import (
	"github.com/hk-32/evie/internal/ast"
	"github.com/hk-32/evie/internal/core"
	"github.com/hk-32/evie/internal/parser"
	"github.com/hk-32/evie/std"
	"github.com/hk-32/evie/std/builtin"
	"github.com/hk-32/evie/std/fs"
	"github.com/hk-32/evie/std/time"
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

func DefaultExports() map[string]core.Value {
	std.Exports = map[string]core.Value{}
	fs.Export()
	time.Export()
	builtin.Export()
	return std.Exports
}

func Setup(opts Options) {
	ast.Setup(opts.Optimise, opts.Exports)
}

func Reset() {

}

func FeedCode(input []byte) (core.Value, error) {
	output, err := parser.Parse(input)
	if err != nil {
		return core.Value{}, err
	}

	return ast.Feed(output)
}

func GetGlobal(name string) *core.Value {
	return ast.GetGlobal(name)
}

func WaitForNoActivity(name string) {
	core.WaitForNoActivity()
}
