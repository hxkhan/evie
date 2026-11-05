package io

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	goio "io"
	"os"
	"strings"

	"github.com/hxkhan/evie/vm"
)

type lineResult struct {
	line string
	err  error
}

var (
	stdinSem     = make(chan struct{}, 1)
	stdinFile    *os.File
	stdinBuf     *bufio.Reader
	stdinPending chan lineResult
)

func readLine(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", context.Cause(ctx)
	}
	select {
	case stdinSem <- struct{}{}:
	case <-ctx.Done():
		return "", context.Cause(ctx)
	}
	defer func() { <-stdinSem }()

	if stdinBuf == nil || stdinFile != os.Stdin {
		stdinFile = os.Stdin
		stdinBuf = bufio.NewReader(os.Stdin)
		stdinPending = nil
	}
	if stdinPending == nil {
		ch := make(chan lineResult, 1)
		stdinPending = ch
		buf := stdinBuf
		go func() {
			line, err := buf.ReadString('\n')
			if err != nil && !errors.Is(err, goio.EOF) {
				ch <- lineResult{err: err}
				return
			}
			ch <- lineResult{line: strings.TrimRight(line, "\r\n")}
		}()
	}

	select {
	case res := <-stdinPending:
		stdinPending = nil
		return res.line, res.err
	case <-ctx.Done():
		return "", context.Cause(ctx)
	}
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
		input, err := readLine(c.Context())
		if err != nil {
			return vm.Value{}, err
		}
		return vm.BoxString(input), nil
	}).Blocking = true
	pkg.Func("readln", 0, func(c vm.Call) (vm.Value, error) {
		input, err := readLine(c.Context())
		if err != nil {
			return vm.Value{}, err
		}
		return vm.BoxString(input), nil
	}).Blocking = true
	return pkg
}
