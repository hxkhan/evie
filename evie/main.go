package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hxkhan/evie"
	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/vm"
)

// report prints an error the way a compiler does and exits
func report(err error) {
	palette := diag.Palette{}
	if info, statErr := os.Stderr.Stat(); statErr == nil && info.Mode()&os.ModeCharDevice != 0 && os.Getenv("NO_COLOR") == "" {
		palette = diag.Colors
	}
	var reporter interface{ Diagnostic() *diag.Report }
	if errors.As(err, &reporter) {
		fmt.Fprint(os.Stderr, reporter.Diagnostic().Render(palette))
	} else {
		fmt.Fprintln(os.Stderr, "error:", err)
	}
	os.Exit(1)
}

func main() {
	inline := flag.Bool("inline", true, "Optimise the program by inlining certain instruction combinations")
	t := flag.Bool("t", false, "Print execution time")
	logCaptures := flag.Bool("log-captures", false, "Log when and what is captured")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Println("Provide a file with a .ev extension or a project directory as the last argument!")
		return
	}

	target := flag.Arg(flag.NArg() - 1)

	eng := evie.New(evie.Options{
		NoInline:    !*inline,
		LogCaptures: *logCaptures,
	})

	before := time.Now()
	var res vm.Value
	var err error

	info, statErr := os.Stat(target)
	if statErr == nil && info.IsDir() {
		res, err = eng.RunProject(target)
	} else if strings.HasSuffix(target, ".ev") || statErr == nil {
		res, err = eng.RunFile(target)
	} else {
		fmt.Println("Provide a file with a .ev extension or a project directory!")
		return
	}

	difference := time.Since(before)
	if err != nil {
		report(err)
	}

	if !res.IsNil() {
		fmt.Println(res)
	}

	if *t {
		fmt.Println("------------------------------")
		fmt.Printf("Execution time: %v\n", difference)
	}
}
