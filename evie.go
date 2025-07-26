package evie

import (
	"io"
	"log"

	"github.com/hxkhan/evie/parser"
	"github.com/hxkhan/evie/std"
	"github.com/hxkhan/evie/std/builtin"
	"github.com/hxkhan/evie/std/fs"
	"github.com/hxkhan/evie/std/time"
	"github.com/hxkhan/evie/vm"
)

type Options struct {
	vm.Options
	DebugLogs bool // print debug logs
}

var Defaults = Options{
	Options: vm.Options{
		Inline:   true,
		Builtins: DefaultExports(),
	},
}

type Interpreter struct {
	*vm.Instance
}

func DefaultExports() map[string]vm.Value {
	std.Exports = map[string]vm.Value{}
	fs.Export()
	time.Export()
	builtin.Export()
	return std.Exports
}

func New(opts Options) *Interpreter {
	log.SetFlags(0)
	if !opts.DebugLogs {
		log.SetOutput(io.Discard)
	}
	m := vm.New(opts.Options)

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

	return ip.EvalNode(output)
}
