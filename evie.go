// contains package level state which means you cannot have multiple instance of evie running at the same time,
// this was done for performance reasons and for a lack of a better way to make concurrency work with struct level state
package evie

import (
	"github.com/hk-32/evie/ast"
	"github.com/hk-32/evie/core"
	"github.com/hk-32/evie/parser"
	"github.com/hk-32/evie/std"
	"github.com/hk-32/evie/std/builtin"
	"github.com/hk-32/evie/std/fs"
	"github.com/hk-32/evie/std/time"
)

type Options struct {
	Optimise      bool // use specialised instructions
	ObserveIt     bool // collect metrics (affects performance)
	TopLevelLogic bool // whether to only allow declarations at top level

	Exports map[string]core.Value // what should be made available to the user
}

var Defaults = Options{Optimise: true, Exports: DefaultExports()}

type Interpreter struct {
	cs *ast.CompilerState
	vm *core.Machine
}

func DefaultExports() map[string]core.Value {
	std.Exports = map[string]core.Value{}
	fs.Export()
	time.Export()
	builtin.Export()
	return std.Exports
}

func New(opts Options) *Interpreter {
	cs := ast.NewCompiler(opts.Exports)
	vm := cs.GetVM()

	/* if opts.ObserveIt {
		core.WrapInstructions(func(rt *core.CoRoutine) {

		}, func(rt *core.CoRoutine) {

		})
	} */

	return &Interpreter{cs, vm}
}

func (ip *Interpreter) Feed(input []byte) (core.Value, error) {
	output, err := parser.Parse(input)
	if err != nil {
		return core.Value{}, err
	}

	return ip.cs.Compile(output)
}

// GetGlobal retrieves a global variable by its name and returns a pointer to it.
// If the global variable does not exist, it returns nil.
func (ip *Interpreter) GetGlobal(name string) *core.Value {
	addr, exists := ip.cs.GetGlobalAddress(name)
	if !exists {
		return nil
	}
	return ip.vm.GetGlobal(addr)
}

func (ip *Interpreter) WaitForNoActivity() {
	ip.cs.GetVM().WaitForNoActivity()
}

func (ip *Interpreter) DumpCode() {
	ip.cs.GetVM().DumpCode()
}

func PrintInstructionStats() {
	core.PrintInstructionRuns()
}
