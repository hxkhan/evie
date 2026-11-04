package io

import (
	"bufio"
	"errors"
	"fmt"
	goio "io"
	"os"
	"strings"
	"sync"

	"github.com/hxkhan/evie/vm"
)

var (
	stdinMu   sync.Mutex
	stdinFile *os.File
	stdinBuf  *bufio.Reader
)

func readLine() (string, error) {
	stdinMu.Lock()
	defer stdinMu.Unlock()

	if stdinBuf == nil || stdinFile != os.Stdin {
		stdinFile = os.Stdin
		stdinBuf = bufio.NewReader(os.Stdin)
	}
	line, err := stdinBuf.ReadString('\n')
	if err != nil && !errors.Is(err, goio.EOF) {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// Construct builds the io package: console output and input
func Construct() *vm.Package {
	pkg := vm.NewPackage("io")
	pkg.Variadic("print", 0, func(c vm.Call) (vm.Value, error) {
		for i := 0; i < c.Len(); i++ {
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Print(c.Arg(i))
		}
		return vm.Value{}, nil
	})
	pkg.Variadic("println", 0, func(c vm.Call) (vm.Value, error) {
		for i := 0; i < c.Len(); i++ {
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Print(c.Arg(i))
		}
		fmt.Println()
		return vm.Value{}, nil
	})
	pkg.Func("prompt", 1, func(c vm.Call) (vm.Value, error) {
		fmt.Print(c.Arg(0))
		input, err := readLine()
		if err != nil {
			return vm.Value{}, err
		}
		return vm.BoxString(input), nil
	}).Blocking = true
	pkg.Func("readln", 0, func(c vm.Call) (vm.Value, error) {
		input, err := readLine()
		if err != nil {
			return vm.Value{}, err
		}
		return vm.BoxString(input), nil
	}).Blocking = true
	return pkg
}
