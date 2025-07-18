// contains package level state which means you cannot have multiple instance of evie running at the same time,
// this was done for performance reasons and for a lack of a better way to make concurrency work with struct level state
package evie

import (
	"github.com/hxkhan/evie/parser"
	"github.com/hxkhan/evie/std"
	"github.com/hxkhan/evie/std/builtin"
	"github.com/hxkhan/evie/std/fs"
	"github.com/hxkhan/evie/std/time"
	"github.com/hxkhan/evie/vm"
)

type Options struct {
	Optimise      bool // use specialised instructions
	ObserveIt     bool // collect metrics (affects performance)
	TopLevelLogic bool // whether to only allow declarations at top level

	BuiltIns map[string]vm.Value // what should be made available to the user in the built-in scope
	Globals  map[string]vm.Value // what should be made available to the user in the global scope
}

var Defaults = Options{Optimise: true, BuiltIns: DefaultExports()}

type Interpreter struct {
	vm *vm.Instance
}

func DefaultExports() map[string]vm.Value {
	std.Exports = map[string]vm.Value{}
	fs.Export()
	time.Export()
	builtin.Export()
	return std.Exports
}

func New(opts Options) *Interpreter {
	m := vm.New(opts.BuiltIns, opts.Optimise)

	/* if opts.ObserveIt {
		vm.WrapInstructions(func(rt *vm.CoRoutine) {

		}, func(rt *vm.CoRoutine) {

		})
	} */

	return &Interpreter{m}
}

func (ip *Interpreter) EvalScript(input []byte) (vm.Value, error) {
	output, err := parser.Parse(input)
	if err != nil {
		return vm.Value{}, err
	}

	return ip.vm.EvalNode(output)
}

// GetGlobal retrieves a global variable by its name and returns a pointer to it.
// If the global variable does not exist, it returns nil.
func (ip *Interpreter) GetGlobal(name string) *vm.Value {
	return ip.vm.GetGlobal(name)
}

func (ip *Interpreter) WaitForNoActivity() {
	ip.vm.WaitForNoActivity()
}
