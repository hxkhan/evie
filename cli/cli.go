package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hk-32/evie"
)

func main() {
	p := flag.Bool("p", false, "Print the program before running it")
	o := flag.Bool("o", true, "Optimise the program with specialised instructions")
	d := flag.Bool("d", false, "Print debug stats")
	t := flag.Bool("t", false, "Print execution time")
	flag.Parse()

	fileName := os.Args[len(os.Args)-1]
	if !strings.HasSuffix(fileName, ".es") {
		fmt.Println("Provide a file with a .es extension as the last argument!")
		return
	}

	input, err := os.ReadFile(fileName)
	if err != nil {
		panic(err)
	}

	ip := evie.New(evie.Options{Optimise: *o, ObserveIt: *d, Exports: evie.DefaultExports()})
	_, err = ip.Feed(input)
	if err != nil {
		fmt.Println(err)
		return
	}

	if *p {
		ip.DumpCode()
		fmt.Println("------------------------------")
	}

	main := ip.GetGlobal("main")
	if main == nil {
		fmt.Println("Error: program requires a main entry point")
		return
	}

	fn, ok := main.AsUserFn()
	if !ok {
		fmt.Println("Error: program requires main to be a function")
		return
	}

	before := time.Now()
	res, err := fn.Call()
	if err != nil {
		fmt.Println(err)
		return
	}

	ip.WaitForNoActivity()
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

	if *d {
		evie.PrintInstructionStats()
	}
}
