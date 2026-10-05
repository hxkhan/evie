// Command evie runs Evie programs.
//
//	evie file.ev          run a file's main function
//	evie project/         run a project directory's main.ev
//	evie -e 'code'        run code given on the command line
//	evie --version        print the version
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
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
  evie [flags] <file.ev | directory>
  evie [flags] -e <code>

Flags:
`

func main() {
	code := flag.String("e", "", "run `code` given on the command line; standard packages it names are imported for it")
	version := flag.Bool("version", false, "print the version and exit")
	timing := flag.Bool("t", false, "print the execution time")
	inline := flag.Bool("inline", true, "let the compiler use its fast paths; false is for debugging the compiler")
	logCaptures := flag.Bool("log-captures", false, "log when and what closures capture")
	flag.Usage = func() {
		fmt.Fprint(flag.CommandLine.Output(), usage)
		flag.PrintDefaults()
	}
	flag.Parse()

	if *version {
		fmt.Println(versionString())
		return
	}

	eng := evie.New(evie.Options{Debug: evie.Debug{
		NoFastPaths: !*inline,
		LogCaptures: *logCaptures,
	}})

	before := time.Now()
	var res vm.Value
	var err error

	switch {
	case *code != "":
		res, err = runInline(eng, *code)
	case flag.NArg() == 1:
		target := flag.Arg(0)
		if info, statErr := os.Stat(target); statErr == nil && info.IsDir() {
			res, err = eng.RunProject(target)
		} else {
			res, err = eng.RunFile(target)
		}
	default:
		flag.Usage()
		os.Exit(2)
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

// standard lists the standard packages -e imports when the code names them
var standard = []string{"fs", "io", "json", "math", "strings", "time"}

var packageUse = regexp.MustCompile(`\b(` + strings.Join(standard, "|") + `)\.`)

// runInline runs code from the command line. A program that declares its own
// package runs as written; anything else becomes the body of main, returning
// its value when it is a single expression.
func runInline(eng *evie.Engine, code string) (vm.Value, error) {
	if strings.HasPrefix(strings.TrimSpace(code), "package ") {
		return runSource(eng, code)
	}

	used := map[string]bool{}
	for _, m := range packageUse.FindAllStringSubmatch(code, -1) {
		used[m[1]] = true
	}
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

	// parse the code alone first, on the line it will have inside main, so a
	// syntax error points into it rather than at the wrapper
	node, err := parser.ParseSource(diag.NewSource("-e", []byte("\n"+code)))
	if err != nil {
		return vm.Value{}, err
	}
	if block, ok := node.(ast.Block); ok && len(block.Code) == 1 && isExpression(block.Code[0]) {
		return runSource(eng, header+"return "+code+"\n}")
	}
	return runSource(eng, header+code+"\n}")
}

// isExpression reports whether a parsed top-level node has a value to print
func isExpression(node ast.Node) bool {
	switch node := node.(type) {
	case ast.Decl, ast.Assign, ast.MutableBinOp, ast.Return, ast.Throw, ast.Block, ast.Echo,
		ast.Conditional, ast.While, ast.For, ast.Switch, ast.Break, ast.Continue, ast.StructDefinition:
		return false
	case ast.Fn:
		return node.UsedAsExpr
	}
	return true
}

func runSource(eng *evie.Engine, src string) (vm.Value, error) {
	if _, err := eng.Load("-e", []byte(src)); err != nil {
		return vm.Value{}, err
	}
	main, ok := eng.Lookup("main.main")
	if !ok || !main.IsCallable() {
		return vm.Value{}, errors.New("-e: the program has no main function")
	}
	res, err := main.Call()
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

// report prints an error the way a compiler does and exits
func report(err error) {
	info, statErr := os.Stderr.Stat()
	colors := statErr == nil && info.Mode()&os.ModeCharDevice != 0 && os.Getenv("NO_COLOR") == ""
	fmt.Fprint(os.Stderr, evie.Format(err, colors))
	os.Exit(1)
}
