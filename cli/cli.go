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
	/* f, err := os.Create("cpu.prof")
	if err != nil {
		log.Fatal("could not create CPU profile: ", err)
	}
	defer f.Close() // error handling omitted for example
	if err := pprof.StartCPUProfile(f); err != nil {
		log.Fatal("could not start CPU profile: ", err)
	}
	defer pprof.StopCPUProfile()

	if err != nil {
		panic(err)
	} */

	inline := flag.Bool("inline", true, "Optimise the program by inlining certain instruction combinations")
	m := flag.Bool("m", false, "Print metrics")
	t := flag.Bool("t", false, "Print execution time")
	logCaptures := flag.Bool("log-captures", false, "Log when and what is captured")
	logCache := flag.Bool("log-cache", false, "Log cache hits/misses")
	flag.Parse()

	fileName := os.Args[len(os.Args)-1]
	if !strings.HasSuffix(fileName, ".ev") {
		fmt.Println("Provide a file with a .ev extension as the last argument!")
		return
	}

	input, err := os.ReadFile(fileName)
	if err != nil {
		panic(err)
	}

	evm := vm.New(vm.Options{
		LogCache:        *logCache,
		LogCaptures:     *logCaptures,
		DisableInlining: !(*inline),
		Metrics:         *m,
		//UniversalStatics: evie.ImplicitBuilitins(),
		ImportResolver: resolver,
	})
	_, err = evm.EvalScript(input)
	if err != nil {
		fmt.Println(err)
		return
	}

	pkgMain := evm.GetPackage("main")
	if pkgMain == nil {
		fmt.Println("Error: no main package found")
		return
	}

	symMain, exists := pkgMain.GetSymbol("main")
	if !exists {
		fmt.Println("Error: no main entry point found")
		return
	}

	fn, ok := symMain.AsUserFn()
	if !ok {
		fmt.Println("Error: main.main found but it is not a function")
		return
	}

	before := time.Now()
	res, err := fn.Call()
	if err != nil {
		fmt.Println(err)
		return
	}

	evm.WaitForNoActivity()
	difference := time.Since(before)

	if !res.IsNil() {
		fmt.Println(res)
	}

	if *m || *t {
		fmt.Println("------------------------------")
	}

	if *t {
		fmt.Printf("Execution time: %v\n", difference)
	}

	/* if *m {
		fmt.Println("Metrics")
	} */
}

func resolver(name string) vm.Package {
	if constructor, exists := evie.StandardLibraryConstructors[name]; exists {
		return constructor()
	}
	panic(fmt.Errorf("constructor not found for '%v'", name))
}
