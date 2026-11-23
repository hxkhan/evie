// Package io is Evie's console package: print, println, prompt and readln.
package io

import (
	"bufio"
	"context"
	"errors"
	goio "io"
	"os"
	"strings"

	"github.com/hxkhan/evie/vm"
)

type lineResult struct {
	line string
	eof  bool
	err  error
}

// LineReader reads lines from one source for everyone who reads from it: the
// io package of any number of engines, and the host itself, such as a REPL
// reading its own input between scripts. Give it to an engine as
// evie.Options.Stdin and the io package reads through it rather than through
// a buffer of its own, so no reader takes lines meant for another.
//
// A read that is cancelled leaves its line pending, and the next read gets it,
// so no line is lost. A Read of the source itself cannot be cancelled: one
// that never returns keeps its goroutine until the source is closed.
type LineReader struct {
	sem     chan struct{}
	src     goio.Reader // nil means track os.Stdin dynamically
	file    *os.File
	buf     *bufio.Reader
	pending chan lineResult
	rest    []byte // what Read has yet to hand out of the last line
}

// NewLineReader makes a line reader for src
func NewLineReader(src goio.Reader) *LineReader {
	r := &LineReader{
		sem: make(chan struct{}, 1),
		src: src,
	}
	if src != nil {
		r.buf = bufio.NewReader(src)
	}
	return r
}

var defaultStdin = NewLineReader(nil)

// ReadLine reads the next line without its line ending; eof says the source
// has no more. When ctx is done first, the read gives up with ctx's cause.
func (r *LineReader) ReadLine(ctx context.Context) (line string, eof bool, err error) {
	return r.readLine(ctx, "", nil)
}

// Read makes a LineReader an io.Reader, so it can be an engine's Stdin; it
// hands out whole lines, each with a newline
func (r *LineReader) Read(p []byte) (int, error) {
	if len(r.rest) == 0 {
		line, eof, err := r.ReadLine(context.Background())
		if err != nil {
			return 0, err
		}
		if eof {
			return 0, goio.EOF
		}
		r.rest = append([]byte(line), '\n')
	}
	n := copy(p, r.rest)
	r.rest = r.rest[n:]
	return n, nil
}

func (r *LineReader) readLine(ctx context.Context, prompt string, out goio.Writer) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, context.Cause(ctx)
	}
	select {
	case r.sem <- struct{}{}:
	case <-ctx.Done():
		return "", false, context.Cause(ctx)
	}
	defer func() { <-r.sem }()

	if prompt != "" {
		_, _ = goio.WriteString(out, prompt)
	}

	if r.src == nil {
		if r.buf == nil || r.file != os.Stdin {
			r.file = os.Stdin
			r.buf = bufio.NewReader(os.Stdin)
			r.pending = nil
		}
	}
	if r.pending == nil {
		ch := make(chan lineResult, 1)
		r.pending = ch
		buf := r.buf
		go func() {
			line, err := buf.ReadString('\n')
			if err != nil {
				if errors.Is(err, goio.EOF) {
					if line == "" {
						ch <- lineResult{eof: true}
						return
					}
					ch <- lineResult{line: strings.TrimRight(line, "\r\n")}
					return
				}
				ch <- lineResult{err: err}
				return
			}
			ch <- lineResult{line: strings.TrimRight(line, "\r\n")}
		}()
	}

	select {
	case res := <-r.pending:
		r.pending = nil
		return res.line, res.eof, res.err
	case <-ctx.Done():
		return "", false, context.Cause(ctx)
	}
}

func writeArgs(out goio.Writer, c vm.Call, newline bool) {
	var b strings.Builder
	for i := 0; i < c.Len(); i++ {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(c.Arg(i).String())
	}
	if newline {
		b.WriteByte('\n')
	}
	_, _ = goio.WriteString(out, b.String())
}

// Construct builds the io package using os.Stdin and os.Stdout.
func Construct() *vm.Package {
	return ConstructWith(nil, nil)
}

// ConstructWith builds an isolated io package reading from in (or os.Stdin when
// nil) and writing to out (or os.Stdout when nil).
func ConstructWith(in goio.Reader, out goio.Writer) *vm.Package {
	reader := defaultStdin
	if lr, ok := in.(*LineReader); ok {
		reader = lr
	} else if in != nil {
		reader = NewLineReader(in)
	}
	writer := func() goio.Writer {
		if out != nil {
			return out
		}
		return os.Stdout
	}

	pkg := vm.NewPackage("io")
	pkg.Variadic("print", 0, func(c vm.Call) (vm.Value, error) {
		writeArgs(writer(), c, false)
		return vm.Value{}, nil
	})
	pkg.Variadic("println", 0, func(c vm.Call) (vm.Value, error) {
		writeArgs(writer(), c, true)
		return vm.Value{}, nil
	})
	pkg.Func("prompt", 1, func(c vm.Call) (vm.Value, error) {
		input, eof, err := reader.readLine(c.Context(), c.Arg(0).String(), writer())
		if err != nil {
			return vm.Value{}, err
		}
		if eof {
			return vm.Value{}, nil
		}
		return vm.BoxString(input), nil
	}).Blocking = true
	pkg.Func("readln", 0, func(c vm.Call) (vm.Value, error) {
		input, eof, err := reader.readLine(c.Context(), "", writer())
		if err != nil {
			return vm.Value{}, err
		}
		if eof {
			return vm.Value{}, nil
		}
		return vm.BoxString(input), nil
	}).Blocking = true
	// what tools such as editors show for each member
	pkg.Document("print", "print(...values)", "Writes the values separated by spaces.")
	pkg.Document("println", "println(...values)", "Writes the values separated by spaces, then a new line.")
	pkg.Document("prompt", "prompt(message)", "Prints the message and reads a line; nil at the end of input.")
	pkg.Document("readln", "readln()", "Reads a line; nil at the end of input.")
	return pkg
}
