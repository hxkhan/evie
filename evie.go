package evie

import (
	"io"
	"log"

	"github.com/hxkhan/evie/parser"
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
		UStatics: ImplicitBuilitins(),
	},
}

type Interpreter struct {
	*vm.Instance
}

func ImplicitBuilitins() map[string]vm.Value {
	return map[string]vm.Value{
		"time": vm.BoxPackage(vm.NewHostPackage("time", time.Instantiate())),
		"fs":   vm.BoxPackage(vm.NewHostPackage("time", fs.Instantiate())),
	}
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
