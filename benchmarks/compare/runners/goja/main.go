// Command goja runs a JavaScript file with goja, embedded the way a Go host
// would embed it, with console.log writing to stdout
package main

import (
	"fmt"
	"os"

	"github.com/dop251/goja"
)

func main() {
	src, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	vm := goja.New()
	console := vm.NewObject()
	console.Set("log", func(call goja.FunctionCall) goja.Value {
		for i, arg := range call.Arguments {
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Print(arg.String())
		}
		fmt.Println()
		return goja.Undefined()
	})
	vm.Set("console", console)
	if _, err := vm.RunScript(os.Args[1], string(src)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
