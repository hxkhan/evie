//go:build js && wasm

// Command wasm is Evie compiled to WebAssembly for the website's playground.
// It runs inside a Web Worker: the page posts a program, the worker calls
// evieRun, and the program's output, result and errors come back as
// messages. A program that reads input asks the page for each line, which the
// user types into the output, as in a terminal. The page stops a runaway
// program by terminating the worker.
//
//	GOOS=js GOARCH=wasm go build -o site/public/evie.wasm ./site/wasm
package main

import (
	"context"
	goio "io"
	"strings"
	"sync"
	"sync/atomic"
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
	// evieRun(code) runs a program whose input is typed while it runs, as in a
	// terminal; evieRun(code, stdin) gives it all of its input up front
	js.Global().Set("evieRun", js.FuncOf(func(this js.Value, args []js.Value) any {
		code := args[0].String()
		var stdin *string
		if len(args) > 1 && args[1].Type() == js.TypeString {
			s := args[1].String()
			stdin = &s
		}
		// a callback must not block the event loop that time.sleep needs, so the
		// program runs on its own goroutine and reports with messages
		go run(code, stdin)
		return nil
	}))
	// evieInput(line) answers a "read" with a line the user typed; evieEOF()
	// ends the input, as Ctrl+D does in a terminal
	js.Global().Set("evieInput", js.FuncOf(func(this js.Value, args []js.Value) any {
		if t := running.Load(); t != nil {
			t.send(args[0].String()+"\n", false)
		}
		return nil
	}))
	js.Global().Set("evieEOF", js.FuncOf(func(this js.Value, args []js.Value) any {
		if t := running.Load(); t != nil {
			t.send("", true)
		}
		return nil
	}))
	js.Global().Call("postMessage", map[string]any{"type": "ready", "version": version()})
	select {}
}

// running is the input of the program that is running, which evieInput feeds
var running atomic.Pointer[terminal]

// terminal is a program's standard input when it is typed as the program
// runs. A read that finds no line waiting shows what was printed so far,
// posts "read" so the page asks for a line, and waits for it. Only this
// goroutine waits: the event loop keeps running, so the line can arrive.
type terminal struct {
	out   *stream
	mu    sync.Mutex
	queue []string
	eof   bool
	wake  chan struct{}
	rest  string // what Read has yet to hand out of the current line
}

func newTerminal(out *stream) *terminal {
	return &terminal{out: out, wake: make(chan struct{}, 1)}
}

func (t *terminal) Read(p []byte) (int, error) {
	for t.rest == "" {
		t.mu.Lock()
		switch {
		case len(t.queue) > 0:
			t.rest, t.queue = t.queue[0], t.queue[1:]
		case t.eof:
			t.mu.Unlock()
			return 0, goio.EOF
		}
		t.mu.Unlock()
		if t.rest == "" {
			t.out.flush() // a prompt shows before the program waits
			js.Global().Call("postMessage", map[string]any{"type": "read"})
			<-t.wake
		}
	}
	n := copy(p, t.rest)
	t.rest = t.rest[n:]
	return n, nil
}

// send queues a typed line, or ends the input
func (t *terminal) send(line string, eof bool) {
	t.mu.Lock()
	if eof {
		t.eof = true
	} else {
		t.queue = append(t.queue, line)
	}
	t.mu.Unlock()
	select {
	case t.wake <- struct{}{}:
	default:
	}
}

// run evaluates code the way `evie -e` does: a program with a package header
// runs its main, anything else runs in a session that shows its last value
func run(code string, stdin *string) {
	// the program is a file of the playground's file system, so a path import
	// such as "./util" resolves there, as does everything fs reads
	fsys := fstest.MapFS{"playground.ev": {Data: []byte(code)}}
	for name, f := range files {
		fsys[name] = f
	}
	out := newStream()
	var in goio.Reader
	if stdin != nil {
		in = strings.NewReader(*stdin)
	} else {
		t := newTerminal(out)
		running.Store(t)
		defer running.CompareAndSwap(t, nil)
		in = t
	}
	eng := evie.New(evie.Options{
		Stdin:  in,
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
