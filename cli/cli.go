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
	d := flag.Bool("d", false, "Print debug stats")
	t := flag.Bool("t", false, "Print execution time")
	log := flag.Bool("log", false, "Log things for debugging")
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
		PrintLogs:       *log,
		DisableInlining: !(*inline),
		ObserveIt:       *d,
		//UniversalStatics: evie.ImplicitBuilitins(),
		PackageContructors: evie.StandardLibrary(),
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

	if !res.IsNull() {
		fmt.Println(res)
	}

	if *d || *t {
		fmt.Println("------------------------------")
	}

	if *t {
		fmt.Printf("Execution time: %v\n", difference)
	}
}
