// Package evie embeds the Evie scripting language in a Go program.
//
// Create an [Engine] with [New] to evaluate scripts, call Evie functions from Go,
// and expose host packages with [Options]. For multi-file programs or concurrent
// execution across isolated engines, parse once into an immutable [Bundle] with
// [ParseFile], [ParseProject] or [ParseSource]. Use [Format] to render parse,
// compile, and runtime errors with source excerpts and traces.
//
//	eng := evie.New()
//	defer eng.Close()
//	eng.Eval(`package main
//	fn add(a, b) => a + b`)
//	sum, err := eng.Call("main.add", 2, 3)
package evie

import (
	"errors"
	"fmt"
	goio "io"
	iofs "io/fs"
	"path/filepath"
	"strings"

	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/std/fs"
	"github.com/hxkhan/evie/std/io"
	"github.com/hxkhan/evie/std/json"
	"github.com/hxkhan/evie/std/math"
	"github.com/hxkhan/evie/std/time"
	"github.com/hxkhan/evie/vm"
)

// stdlib constructs a standard library package on first import
var stdlib = map[string]func() *vm.Package{
	"io":   io.Construct,
	"fs":   fs.Construct,
	"time": time.Construct,
	"json": json.Construct,
	"math": math.Construct,
}

// Options configures an Engine. The zero value gives the standard library,
// relative imports resolved against the entry file, and no host packages.
type Options struct {
	FS       iofs.FS                                // filesystem for file/project loading, relative imports and the fs package, which can then only read from it; defaults to the OS filesystem
	NoStdlib bool                                   // do not offer the standard library packages
	Stdlib   []string                               // when non-empty, only allow these standard library packages (e.g. "math", "json"); New panics on a name that is not one
	Packages []*vm.Package                          // host packages, importable by their names
	Resolve  func(path string) (*vm.Package, error) // consulted last, for an import by name that no host, standard or loaded package answers; path imports never reach it
	Root     string                                 // where an import that is not relative to its file, such as "lib/x", and a path import in Eval start from; the entry file's directory when empty
	Prelude  *vm.Package                            // symbols visible to every script without an import
	Args     []string                               // arguments passed to main when it accepts parameters
	Stdin    goio.Reader                            // standard input for the io package; defaults to os.Stdin
	Stdout   goio.Writer                            // standard output for the io package and echo; defaults to os.Stdout

	// OnName, when set, receives every name the compiler declares or resolves,
	// with where it is declared, for tools such as editors; see vm.NameEvent.
	// Use it with Check to learn about a program without running it.
	OnName func(vm.NameEvent)

	Debug Debug // switches for working on Evie itself
}

// Debug holds switches for working on Evie itself; hosts leave them off
type Debug struct {
	DebugInfo   bool // record slot names, local classes and nested closures during compilation for Dump
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

	root := opt.Root
	if root != "" && opt.FS == nil {
		if abs, err := filepath.Abs(root); err == nil {
			root = abs
		}
	}

	var stdAllow map[string]bool
	if len(opt.Stdlib) > 0 {
		stdAllow = make(map[string]bool, len(opt.Stdlib))
		for _, name := range opt.Stdlib {
			if _, ok := stdlib[name]; !ok {
				panic(fmt.Sprintf("evie.New: Options.Stdlib names '%s', which is not a standard package", name))
			}
			stdAllow[name] = true
		}
	}

	e := &Engine{
		opts:         opt,
		root:         root,
		stdAllow:     stdAllow,
		packages:     make(map[string]*vm.Package, len(opt.Packages)),
		bundleLoaded: make(map[string]*vm.Package),
	}
	for _, pkg := range opt.Packages {
		if pkg == nil {
			continue
		}
		// clone the package wrapper so per-engine hints never mutate the caller's Package
		cp := pkg.Clone()
		cp.SetID("host:" + pkg.Name())
		e.packages[pkg.Name()] = cp
	}

	e.vm = vm.New(vm.Options{
		Resolver:        e.resolve,
		Prelude:         opt.Prelude,
		Stdout:          opt.Stdout,
		DebugInfo:       opt.Debug.DebugInfo,
		DisableInlining: opt.Debug.NoFastPaths || testWithoutFastPaths,
		OnName:          opt.OnName,
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
	palette := diag.Palette{}
	if colors {
		palette = diag.Colors
	}
	// several syntax errors of one source render one after another
	var list interface{ Diagnostics() []*diag.Report }
	if errors.As(err, &list) {
		reports := list.Diagnostics()
		parts := make([]string, len(reports))
		for i, r := range reports {
			parts[i] = r.Render(palette)
		}
		return strings.Join(parts, "\n")
	}
	var reporter interface{ Diagnostic() *diag.Report }
	if !errors.As(err, &reporter) {
		return "error: " + err.Error() + "\n"
	}
	return reporter.Diagnostic().Render(palette)
}

// RunFile executes a script file in a fresh engine and runs its main function
func RunFile(path string) (vm.Value, error) {
	return New().RunFile(path)
}

// Check compiles a parsed program in a fresh engine without running any of
// it, and returns the compile errors a run would stop with, or nil. Parse with
// ParseFile or ParseProject first; their errors are the syntax errors.
func Check(b *Bundle) error {
	return New().Check(b)
}

// ErrRanAlready is returned when an engine is asked to run a second program
var ErrRanAlready = errors.New("this engine already ran a program; create a new engine for each program")

// ErrClosed is returned by an engine that was closed
var ErrClosed = errors.New("the engine is closed")
