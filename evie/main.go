package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hxkhan/evie"
	"github.com/hxkhan/evie/vm"
)

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
		fmt.Println("Error:", err)
		return
	}

	if !res.IsNil() {
		fmt.Println(res)
	}

	if *t {
		fmt.Println("------------------------------")
		fmt.Printf("Execution time: %v\n", difference)
	}
}
