// Package evie embeds the Evie scripting language in a Go program.
//
//	eng := evie.New()
//	eng.Eval(`package main
//	fn add(a, b) => a + b`)
//	sum, err := eng.Call("main.add", 2, 3)
package evie

import (
	"errors"

	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/std/fs"
	"github.com/hxkhan/evie/std/io"
	"github.com/hxkhan/evie/std/json"
	"github.com/hxkhan/evie/std/math"
	"github.com/hxkhan/evie/std/strings"
	"github.com/hxkhan/evie/std/time"
	"github.com/hxkhan/evie/vm"
)

// stdlib constructs a standard library package on first import
var stdlib = map[string]func() *vm.Package{
	"io":      io.Construct,
	"fs":      fs.Construct,
	"time":    time.Construct,
	"strings": strings.Construct,
	"json":    json.Construct,
	"math":    math.Construct,
}

// Options configures an Engine. The zero value gives the standard library,
// relative imports resolved against the entry file, and no host packages.
type Options struct {
	NoStdlib bool                                   // do not offer the standard library packages
	Packages []*vm.Package                          // host packages, importable by their names
	Resolve  func(path string) (*vm.Package, error) // consulted for an import before the filesystem
	Root     string                                 // directory that relative imports resolve against; the entry file's directory when empty
	Prelude  *vm.Package                            // symbols visible to every script without an import

	Debug Debug // switches for working on Evie itself
}

// Debug holds switches for working on Evie itself; hosts leave them off
type Debug struct {
	LogCaptures bool // log when and what closures capture
	NoFastPaths bool // compile without the compiler's fast paths, to tell whether a bug is in one
}

// testWithoutFastPaths makes every engine compile without the fast paths;
// only the tests set it, to check that the general path gives the same results
var testWithoutFastPaths bool

// New creates an engine. With no options it offers the standard library and
// resolves imports relative to the file being run.
func New(opts ...Options) *Engine {
	var opt Options
	if len(opts) > 0 {
		opt = opts[0]
	}

	e := &Engine{
		opts:         opt,
		root:         opt.Root,
		packages:     make(map[string]*vm.Package, len(opt.Packages)),
		filePackages: make(map[string]*vm.Package),
		dirPackages:  make(map[string]*vm.Package),
		loading:      make(map[string]bool),
	}
	for _, pkg := range opt.Packages {
		e.packages[pkg.Name()] = pkg
	}

	e.Instance = vm.New(vm.Options{
		Resolver:        e.resolve,
		Prelude:         opt.Prelude,
		LogCaptures:     opt.Debug.LogCaptures,
		DisableInlining: opt.Debug.NoFastPaths || testWithoutFastPaths,
	})
	return e
}

// Eval runs a script in a fresh engine and returns its result; ToGo turns it
// into a Go value
func Eval(script string) (vm.Value, error) {
	return New().Eval(script)
}

// Format renders an error the way the evie command prints it: the message, the
// source line with the span underlined, related places, a hint and the trace.
// Colors are for terminals. An error without a report renders as its text.
func Format(err error, colors bool) string {
	if err == nil {
		return ""
	}
	var reporter interface{ Diagnostic() *diag.Report }
	if !errors.As(err, &reporter) {
		return "error: " + err.Error() + "\n"
	}
	palette := diag.Palette{}
	if colors {
		palette = diag.Colors
	}
	return reporter.Diagnostic().Render(palette)
}

// RunFile executes a script file in a fresh engine and runs its main function
func RunFile(path string) (vm.Value, error) {
	return New().RunFile(path)
}

// RunProject executes the main.ev of a project directory in a fresh engine
func RunProject(dir string) (vm.Value, error) {
	return New().RunProject(dir)
}
