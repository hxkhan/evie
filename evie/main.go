// Command evie runs Evie programs.
//
//	evie file.ev          run a file's main function
//	evie project/         run a project directory's main.ev
//	evie -e 'code'        run code given on the command line
//	evie --version        print the version
//
// Ctrl+C stops a running script, which then reports where it stopped; a
// second Ctrl+C exits at once.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/hxkhan/evie"
	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/parser"
	"github.com/hxkhan/evie/vm"
)

const usage = `Usage:
  evie [flags] <file.ev | directory> [args...]
  evie [flags] -e <code> [args...]

Flags:
`

func main() {
	code := flag.String("e", "", "run `code` given on the command line; standard packages it names are imported for it")
	version := flag.Bool("version", false, "print the version and exit")
	flag.BoolVar(version, "v", false, "print the version and exit (short for -version)")
	timing := flag.Bool("t", false, "print the execution time")
	inline := flag.Bool("inline", true, "let the compiler use its fast paths; false is for debugging the compiler")
	dump := flag.Bool("dump", false, "compile and print packages, globals, classes, slots, escapes and captures without running main")
	flag.Usage = func() {
		fmt.Fprint(flag.CommandLine.Output(), usage)
		flag.PrintDefaults()
	}
	flag.Parse()

	if *version {
		fmt.Println(versionString())
		return
	}

	hasCode := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "e" {
			hasCode = true
		}
	})

	var scriptArgs []string
	switch {
	case hasCode:
		scriptArgs = flag.Args()
	case flag.NArg() >= 1:
		scriptArgs = flag.Args()[1:]
	default:
		flag.Usage()
		os.Exit(2)
	}

	eng := evie.New(evie.Options{
		Args: scriptArgs,
		Debug: evie.Debug{
			NoFastPaths: !*inline,
			DebugInfo:   *dump,
		},
	})
	stopOnInterrupt(eng)

	if *dump {
		var err error
		if hasCode {
			err = dumpInline(eng, *code)
		} else {
			target := flag.Arg(0)
			if info, statErr := os.Stat(target); statErr == nil && info.IsDir() {
				err = eng.LoadProject(target)
			} else {
				err = eng.LoadFile(target)
			}
		}
		if err != nil {
			report(err)
		}
		eng.Dump(os.Stdout)
		eng.Close()
		return
	}

	before := time.Now()
	var res vm.Value
	var err error

	if hasCode {
		res, err = runInline(eng, *code, scriptArgs)
	} else {
		target := flag.Arg(0)
		if info, statErr := os.Stat(target); statErr == nil && info.IsDir() {
			res, err = eng.RunProject(target)
		} else {
			res, err = eng.RunFile(target)
		}
	}

	elapsed := time.Since(before)
	if err != nil {
		report(err)
	}
	if !res.IsNil() {
		fmt.Println(res)
	}
	if *timing {
		fmt.Println("------------------------------")
		fmt.Printf("Execution time: %v\n", elapsed)
	}
}

// standard lists the standard packages -e imports when the code references them
var standard = map[string]bool{
	"fs":      true,
	"io":      true,
	"json":    true,
	"math":    true,
	"strings": true,
	"time":    true,
}

// prepareInline wraps bare -e code into package main with auto-detected stdlib imports.
func prepareInline(code string) (string, error) {
	if parser.PeekPackageName(diag.NewSource("-e", []byte(code))) != "" {
		return code, nil
	}

	// parse the code alone first, on the line it will have inside main, so a
	// syntax error points into it rather than at the wrapper
	node, err := parser.ParseSource(diag.NewSource("-e", []byte("\n"+code)))
	if err != nil {
		return "", err
	}

	used := collectStdImports(node)
	imports := make([]string, 0, len(used))
	for name := range used {
		imports = append(imports, fmt.Sprintf("%q", name))
	}
	sort.Strings(imports)

	// the user's code starts on line 2, column 1 or 8, so positions in errors
	// point into it
	header := "package main"
	if len(imports) > 0 {
		header += " imports(" + strings.Join(imports, ", ") + ")"
	}
	header += " fn main() {\n"

	if block, ok := node.(ast.Block); ok && len(block.Code) == 1 && isExpression(block.Code[0]) {
		return header + "return " + code + "\n}", nil
	}
	return header + code + "\n}", nil
}

// runInline runs code from the command line. A program that declares its own
// package runs as written; anything else becomes the body of main, returning
// its value when it is a single expression.
func runInline(eng *evie.Engine, code string, scriptArgs []string) (vm.Value, error) {
	src, err := prepareInline(code)
	if err != nil {
		return vm.Value{}, err
	}
	return runSource(eng, src, scriptArgs)
}

func dumpInline(eng *evie.Engine, code string) error {
	src, err := prepareInline(code)
	if err != nil {
		return err
	}
	_, err = eng.Load("-e", []byte(src))
	return err
}

// collectStdImports walks the parsed AST to find standard library packages
// referenced as `pkg.member`, ignoring strings, comments, field names and
// locally shadowed bindings.
func collectStdImports(root ast.Node) map[string]bool {
	used := map[string]bool{}
	declared := map[string]bool{}

	var collectDecls func(n ast.Node)
	collectDecls = func(n ast.Node) {
		switch n := n.(type) {
		case ast.Block:
			for _, s := range n.Code {
				collectDecls(s)
			}
		case ast.Decl:
			declared[n.Name] = true
		case ast.Fn:
			if n.Name != "" {
				declared[n.Name] = true
			}
		case ast.ClassDefinition:
			if n.Name != "" {
				declared[n.Name] = true
			}
		case ast.For:
			if n.IndexVar != "" {
				declared[n.IndexVar] = true
			}
			if n.ItemVar != "" {
				declared[n.ItemVar] = true
			}
		}
	}
	collectDecls(root)

	ast.Walk(root, func(n ast.Node) bool {
		if fa, ok := n.(ast.FieldAccess); ok {
			if id, ok := fa.Lhs.(ast.Ident); ok && standard[id.Name] && !declared[id.Name] {
				used[id.Name] = true
				return false
			}
		}
		return true
	})
	return used
}

// isExpression reports whether a parsed top-level node has a value to print
func isExpression(node ast.Node) bool {
	switch node := node.(type) {
	case ast.Decl, ast.Assign, ast.MutableBinOp, ast.Return, ast.Throw, ast.Block, ast.Echo,
		ast.Conditional, ast.While, ast.For, ast.Switch, ast.Break, ast.Continue, ast.ClassDefinition:
		return false
	case ast.Fn:
		return node.UsedAsExpr
	}
	return true
}

func runSource(eng *evie.Engine, src string, scriptArgs []string) (vm.Value, error) {
	if _, err := eng.Load("-e", []byte(src)); err != nil {
		return vm.Value{}, err
	}
	mainFn, ok := eng.Lookup("main.main")
	if !ok || !mainFn.IsCallable() {
		eng.Close()
		return vm.Value{}, nil
	}
	var args []any
	if len(scriptArgs) > 0 && mainFn.AcceptsArgs() {
		args = make([]any, len(scriptArgs))
		for i, arg := range scriptArgs {
			args[i] = arg
		}
	}
	res, err := eng.Call("main.main", args...)
	eng.Close()
	return res, err
}

// versionString names the build: the module version when installed with go
// install, otherwise the commit it was built from
func versionString() string {
	version := "devel"
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			version = v
		} else {
			var revision, modified string
			for _, s := range info.Settings {
				switch s.Key {
				case "vcs.revision":
					revision = s.Value
				case "vcs.modified":
					if s.Value == "true" {
						modified = "+dirty"
					}
				}
			}
			if len(revision) >= 7 {
				version = "devel " + revision[:7] + modified
			}
		}
	}
	return fmt.Sprintf("evie %s (%s %s/%s)", version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

// stopOnInterrupt makes the first Ctrl+C stop the script, so it reports where
// it was, and a second one exit at once, for a script stuck in a native that
// cannot be woken (reading stdin, say)
func stopOnInterrupt(eng *evie.Engine) {
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt)
	go func() {
		<-signals
		eng.Interrupt()
		<-signals
		os.Exit(130)
	}()
}

// report prints an error the way a compiler does and exits, with 130 for a
// script stopped by Ctrl+C, as shells expect
func report(err error) {
	info, statErr := os.Stderr.Stat()
	colors := statErr == nil && info.Mode()&os.ModeCharDevice != 0 && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
	fmt.Fprint(os.Stderr, evie.Format(err, colors))
	if errors.Is(err, vm.ErrInterrupted) {
		os.Exit(130)
	}
	os.Exit(1)
}
