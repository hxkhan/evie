package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hk-32/evie"
	"github.com/hk-32/evie/internal/ast"
	"github.com/hk-32/evie/internal/core"
	"github.com/hk-32/evie/internal/parser"
)

func main() {
	//p := flag.Bool("p", false, "To print the program before running it")
	o := flag.Bool("o", true, "To optimise the program with specialised instructions")
	d := flag.Bool("d", false, "To print debug stats")
	t := flag.Bool("t", false, "Print time to run")
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

	pack, err := parser.Parse(input)
	if err != nil {
		fmt.Println(err)
		return
	}

	ast.Setup(*o, evie.DefaultExports())

	/* if *p {
		program.PrintCode()
		fmt.Println("------------------------------")
	} */

	before := time.Now()
	_, err = ast.Feed(pack)
	if err != nil {
		fmt.Println(err)
		return
	}

	main := ast.GetGlobal("main")
	if main == nil {
		fmt.Println("Error: program requires a main entry point")
		return
	}

	fn, ok := main.AsUserFn()
	if !ok {
		fmt.Println("Error: program requires main to be a function")
		return
	}

	res, err := fn.Call()
	if err != nil {
		fmt.Println(err)
		return
	}

	core.WaitForNoActivity()

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
