package evie_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing/fstest"
	"time"

	"github.com/hxkhan/evie"
	"github.com/hxkhan/evie/vm"
)

func ExampleEval() {
	val, err := evie.Eval("return [1, 2, 3, 4].filter(fn(x) => x % 2 == 0).map(fn(x) => x * 10)")
	if err != nil {
		panic(err)
	}
	fmt.Println(val)
	// Output:
	// [20, 40]
}

func ExampleEngine_Call() {
	api := vm.NewPackage("api")
	api.Native("double", vm.Func1(func(n int) int { return n * 2 }))

	eng := evie.New(evie.Options{Packages: []*vm.Package{api}})
	defer eng.Close()

	_, err := eng.Eval(`package main imports("api")
		fn compute(name, n) {
			return { name: name, value: api.double(n) + 1 }
		}`)
	if err != nil {
		panic(err)
	}

	val, err := eng.Call("main.compute", "score", 20)
	if err != nil {
		panic(err)
	}

	var out struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}
	if err := val.Unmarshal(&out); err != nil {
		panic(err)
	}
	fmt.Printf("%s = %d\n", out.Name, out.Value)
	// Output:
	// score = 41
}

func ExampleParseProjectFS() {
	fsys := fstest.MapFS{
		"main.ev": &fstest.MapFile{
			Data: []byte(`package main imports("io", "./util" as u)
				fn main() {
					io.println(u.greet("world"))
					return u.greet("evie")
				}`),
		},
		"util.ev": &fstest.MapFile{
			Data: []byte(`package util
				fn greet(name) => "hello, " + name`),
		},
	}

	// Parse once into an immutable Bundle, then run in a sandboxed Engine.
	bundle, err := evie.ParseProjectFS(fsys, ".")
	if err != nil {
		panic(err)
	}

	var stdout bytes.Buffer
	val, err := bundle.Run(evie.Options{
		Stdlib: []string{"io"},
		Stdout: &stdout,
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("stdout:", strings.TrimSpace(stdout.String()))
	fmt.Println("return:", val)
	// Output:
	// stdout: hello, world
	// return: hello, evie
}

func ExampleEngine_CallContext() {
	eng := evie.New(evie.Options{NoStdlib: true})
	defer eng.Close()

	_, err := eng.Eval(`package main
		fn spin() {
			while true {}
		}`)
	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err = eng.CallContext(ctx, "main.spin")
	fmt.Println("timed out:", errors.Is(err, context.DeadlineExceeded))
	// Output:
	// timed out: true
}

func ExampleFormat() {
	_, err := evie.Eval("x := 1\nx = 2")
	fmt.Println(strings.Contains(evie.Format(err, false), "cannot assign to 'x'"))
	// Output:
	// true
}
