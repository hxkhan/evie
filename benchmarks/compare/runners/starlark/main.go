// Command starlark runs a Starlark file with starlark-go, embedded the way a
// Go host would embed it, with print writing to stdout. While loops and
// recursion are allowed, as the benchmarks need them.
package main

import (
	"fmt"
	"os"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

func main() {
	thread := &starlark.Thread{Print: func(_ *starlark.Thread, msg string) { fmt.Println(msg) }}
	opts := &syntax.FileOptions{While: true, Recursion: true, TopLevelControl: true, GlobalReassign: true}
	if _, err := starlark.ExecFileOptions(opts, thread, os.Args[1], nil, nil); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
