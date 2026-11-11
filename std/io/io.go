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

// lineReader serializes line reads from a single io.Reader and keeps an
// in-flight read cancellable via context.
type lineReader struct {
	sem     chan struct{}
	src     goio.Reader // nil means track os.Stdin dynamically
	file    *os.File
	buf     *bufio.Reader
	pending chan lineResult
}

func newLineReader(src goio.Reader) *lineReader {
	r := &lineReader{
		sem: make(chan struct{}, 1),
		src: src,
	}
	if src != nil {
		r.buf = bufio.NewReader(src)
	}
	return r
}

var defaultStdin = newLineReader(nil)

func (r *lineReader) readLine(ctx context.Context, prompt string, out goio.Writer) (string, bool, error) {
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
	if in != nil {
		reader = newLineReader(in)
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
	return pkg
}
