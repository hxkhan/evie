//go:build js && wasm

// Command wasm is Evie compiled to WebAssembly for the website's playground.
// It runs inside a Web Worker: the page posts a program, the worker calls
// evieRun, and the program's output, result and errors come back as
// messages. The page stops a runaway program by terminating the worker.
//
//	GOOS=js GOARCH=wasm go build -o site/public/evie.wasm ./site/wasm
package main

import (
	"context"
	"strings"
	"sync"
	"syscall/js"
	"testing/fstest"
	"time"

	"github.com/hxkhan/evie"
	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/parser"
	"github.com/hxkhan/evie/vm"
)

// files is the read-only file system scripts see through the fs package
var files = fstest.MapFS{
	"hello.txt":      {Data: []byte("Hello from the playground's file system!\n")},
	"data/nums.json": {Data: []byte("[3, 1, 4, 1, 5, 9, 2, 6]\n")},
}

func main() {
	js.Global().Set("evieRun", js.FuncOf(func(this js.Value, args []js.Value) any {
		code, stdin := args[0].String(), ""
		if len(args) > 1 {
			stdin = args[1].String()
		}
		// a callback must not block the event loop that time.sleep needs, so the
		// program runs on its own goroutine and reports with messages
		go run(code, stdin)
		return nil
	}))
	js.Global().Call("postMessage", map[string]any{"type": "ready", "version": version()})
	select {}
}

// run evaluates code the way `evie -e` does: a program with a package header
// runs its main, anything else runs in a session that shows its last value
func run(code, stdin string) {
	// the program is a file of the playground's file system, so a path import
	// such as "./util" resolves there, as does everything fs reads
	fsys := fstest.MapFS{"playground.ev": {Data: []byte(code)}}
	for name, f := range files {
		fsys[name] = f
	}
	out := newStream()
	eng := evie.New(evie.Options{
		Stdin:  strings.NewReader(stdin),
		Stdout: out,
		FS:     fsys,
	})
	defer eng.Close()

	start := time.Now()
	var val vm.Value
	var hasVal bool
	var err error
	if parser.PeekPackageName(diag.NewSource("playground.ev", []byte(code))) != "" {
		var b *evie.Bundle
		if b, err = evie.ParseFile(fsys, "playground.ev"); err == nil {
			val, err = eng.RunBundle(context.Background(), b)
			hasVal = !val.IsNil()
		}
	} else {
		val, hasVal, err = eng.NewSession().Eval(code)
		eng.Wait()
	}
	elapsed := time.Since(start)
	out.flush()

	msg := map[string]any{"type": "done", "ms": float64(elapsed.Microseconds()) / 1000}
	if err != nil {
		msg["error"] = evie.Format(err, false)
	} else if hasVal && !val.IsNil() {
		msg["result"] = val.String()
	}
	js.Global().Call("postMessage", msg)
}

// stream batches what a program prints and posts it to the page every few
// milliseconds, so a loop of prints does not flood the message queue
type stream struct {
	mu    sync.Mutex
	buf   strings.Builder
	timer *time.Timer
}

func newStream() *stream { return &stream{} }

func (s *stream) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf.Write(p)
	if s.buf.Len() > 64<<10 {
		s.flushLocked()
	} else if s.timer == nil {
		s.timer = time.AfterFunc(30*time.Millisecond, s.flush)
	}
	return len(p), nil
}

func (s *stream) flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flushLocked()
}

func (s *stream) flushLocked() {
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if s.buf.Len() == 0 {
		return
	}
	js.Global().Call("postMessage", map[string]any{"type": "out", "text": s.buf.String()})
	s.buf.Reset()
}

// version names the build the way `evie --version` does
func version() string {
	return "evie (wasm)"
}
