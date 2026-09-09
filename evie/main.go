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
	m := flag.Bool("m", false, "Print metrics")
	t := flag.Bool("t", false, "Print execution time")
	logCaptures := flag.Bool("log-captures", false, "Log when and what is captured")
	logCache := flag.Bool("log-cache", false, "Log cache hits/misses")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Println("Provide a file with a .ev extension or a project directory as the last argument!")
		return
	}

	target := flag.Arg(flag.NArg() - 1)

	loader := evie.NewLoader(evie.LoaderOptions{
		Inline:      *inline,
		Metrics:     *m,
		LogCaptures: *logCaptures,
		LogCache:    *logCache,
	})

	before := time.Now()
	var res vm.Value
	var err error

	info, statErr := os.Stat(target)
	if statErr == nil && info.IsDir() {
		res, err = loader.RunProject(target)
	} else if strings.HasSuffix(target, ".ev") || statErr == nil {
		res, err = loader.RunFile(target)
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

	if *m || *t {
		fmt.Println("------------------------------")
	}

	if *t {
		fmt.Printf("Execution time: %v\n", difference)
	}
}
