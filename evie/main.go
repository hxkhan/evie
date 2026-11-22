// Command evie runs Evie programs or starts an interactive REPL.
//
//	evie                  start an interactive REPL
//	evie file.ev          run a file's main function
//	evie project/         run the package main files of a project directory
//	evie -e 'code'        run code given on the command line
//	evie --version        print the version
//
// Ctrl+C stops a running script, which then reports where it stopped; a
// second Ctrl+C exits at once.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/hxkhan/evie"
	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/parser"
	"github.com/hxkhan/evie/vm"
)

const usage = `Usage:
  evie [flags]                         start an interactive REPL
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
		if *dump {
			fmt.Fprintln(os.Stderr, "error: -dump requires a file, directory, or -e code (or use :dump inside the REPL)")
			os.Exit(2)
		}
		startREPL(*inline, *timing)
		return
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
			err = dumpTarget(eng, flag.Arg(0))
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
		res, err = runInline(eng, *code)
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

// runInline runs code from the command line. A program that declares its own
// package runs as written (calling main.main with the engine's Args); anything
// else runs in a session with automatic stdlib imports and gives its trailing
// expression value.
func runInline(eng *evie.Engine, code string) (vm.Value, error) {
	if !hasPackageHeader(code) {
		val, _, err := eng.NewSession().Eval(code)
		eng.Wait()
		return val, err
	}
	b, err := evie.ParseSource("-e", []byte(code))
	if err != nil {
		return vm.Value{}, err
	}
	return eng.RunBundle(context.Background(), b)
}

func dumpInline(eng *evie.Engine, code string) error {
	if !hasPackageHeader(code) {
		return eng.NewSession().Compile(code)
	}
	b, err := evie.ParseSource("-e", []byte(code))
	if err != nil {
		return err
	}
	return eng.LoadBundle(context.Background(), b)
}

// dumpTarget compiles a file or project directory without running main
func dumpTarget(eng *evie.Engine, target string) error {
	var b *evie.Bundle
	var err error
	if info, statErr := os.Stat(target); statErr == nil && info.IsDir() {
		b, err = evie.ParseProject(nil, target)
	} else {
		b, err = evie.ParseFile(nil, target)
	}
	if err != nil {
		return err
	}
	return eng.LoadBundle(context.Background(), b)
}

// hasPackageHeader reports whether code starts with a `package <name>` header,
// skipping whitespace and comments the way the parser does
func hasPackageHeader(code string) bool {
	return parser.PeekPackageName(diag.NewSource("-e", []byte(code))) != ""
}

// buildVersion is set at release build time via -ldflags "-X main.buildVersion=..."
var buildVersion string

// versionString names the build: an injected release version, the module
// version when installed with go install, or the commit it was built from.
func versionString() string {
	version := buildVersion
	if version == "" {
		version = "devel"
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
