package main

import (
	"fmt"
	"os"
	"time"

	"github.com/hk-32/evie"
	"github.com/hk-32/evie/core"
	"github.com/hk-32/evie/internal/ast"
	"github.com/hk-32/evie/internal/parser"
)

func main() {
	fileName := "./fib.es"
	optimise := true
	observe := true
	print := false
	measure := true

	input, err := os.ReadFile(fileName)
	if err != nil {
		panic(err)
	}

	pack, err := parser.Parse(input)
	if err != nil {
		fmt.Println(err)
		return
	}

	routine, err := ast.Compile(pack, optimise, evie.DefaultExports())
	if err != nil {
		fmt.Println(err)
		return
	}

	if print {
		routine.PrintCode()
		fmt.Println("------------------------------")
	}

	before := time.Now()
	err = routine.Initialize()
	if err != nil {
		fmt.Println(err)
		return
	}

	main := core.GetGlobal("main")
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

	difference := time.Since(before)

	if !res.IsNull() {
		fmt.Println(res)
	}

	if observe || measure {
		fmt.Println("------------------------------")
	}

	if measure {
		fmt.Printf("Execution time: %v\n", difference)
	}

	/* if *d {
		program.PrintStats()
	} */
}

/* func main() {
	p := flag.Bool("p", false, "To print the program before running it")
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

	pack, err := parser.ParsePackage(input)
	if err != nil {
		fmt.Println(err)
		return
	}

	//pack

	program, err := evie.NewProgramFromAST(pack, *o, *d)
	if err != nil {
		fmt.Println(err)
		return
	}

	if *p {
		program.PrintCode()
		fmt.Println("------------------------------")
	}

	before := time.Now()
	res, err := program.Start()
	difference := time.Since(before)

	if err != nil {
		fmt.Println(err)
		return
	}

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
*/
