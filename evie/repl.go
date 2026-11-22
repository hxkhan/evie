package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hxkhan/evie"
	evieio "github.com/hxkhan/evie/std/io"
)

type replConfig struct {
	inline      bool
	timing      bool
	interactive bool
	colors      bool
}

const replHelp = `REPL commands:
  :help, :h                    show this help
  :vars, :env                  list defined variables, functions, classes and imports
  :import <path> [as <alias>]  import a standard or local package into this session
  :load <file.ev>              evaluate a file into this session
  :dump                        dump compiled functions, classes, slots and captures
  :reset, :clear               clear all session state
  :quit, :exit, :q             exit the REPL

Tips:
  - Standard library packages (io, fs, json, math, time) auto-import on use.
  - The last non-nil expression result is saved in _.
  - Multi-line blocks, arrays, objects, templates and => arrows continue automatically.
  - Press Ctrl+C while code is running to interrupt it without exiting the REPL.
`

func startREPL(inline, timing bool) {
	inTTY := isCharDevice(os.Stdin)
	outColors := isCharDevice(os.Stdout) && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
	cfg := replConfig{
		inline:      inline,
		timing:      timing,
		interactive: inTTY,
		colors:      outColors,
	}

	var activeEngine atomic.Pointer[evie.Engine]
	var running atomic.Bool
	var interrupted atomic.Bool

	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt)
	go func() {
		for range signals {
			if running.Load() {
				if interrupted.Swap(true) {
					os.Exit(130)
				}
				if eng := activeEngine.Load(); eng != nil {
					eng.Interrupt()
				}
				continue
			}
			fmt.Fprintln(os.Stdout)
			os.Exit(0)
		}
	}()
	defer signal.Stop(signals)

	runREPLLoop(os.Stdin, os.Stdout, os.Stderr, cfg, &activeEngine, &running, &interrupted)
}

func runREPLLoop(
	in io.Reader,
	out, errOut io.Writer,
	cfg replConfig,
	activeEngine *atomic.Pointer[evie.Engine],
	running, interrupted *atomic.Bool,
) {
	// the REPL and the scripts it runs read stdin through one line reader, so a
	// script's io.readln gets the next line, not one the REPL read ahead
	lines := evieio.NewLineReader(in)
	newSession := func() (*evie.Engine, *evie.Session) {
		eng := evie.New(evie.Options{
			Root:   ".",
			Stdin:  lines,
			Stdout: out,
			Debug: evie.Debug{
				NoFastPaths: !cfg.inline,
				DebugInfo:   true,
			},
		})
		if activeEngine != nil {
			activeEngine.Store(eng)
		}
		return eng, eng.NewSession()
	}

	eng, sess := newSession()
	defer func() {
		eng.Close()
		if activeEngine != nil {
			activeEngine.Store(nil)
		}
	}()

	if cfg.interactive {
		fmt.Fprintf(out, "%s\n", versionString())
		fmt.Fprintln(out, "Type :help for commands, :quit or Ctrl+D to exit.")
	}

	promptMain := "evie> "
	promptCont := "  ... "
	if cfg.colors {
		promptMain = "\033[1;36mevie>\033[0m "
		promptCont = "\033[90m  ...\033[0m "
	}

	var buf strings.Builder
	for {
		if cfg.interactive {
			if buf.Len() == 0 {
				fmt.Fprint(out, promptMain)
			} else {
				fmt.Fprint(out, promptCont)
			}
		}

		line, eof, err := lines.ReadLine(context.Background())
		if eof || err != nil {
			if buf.Len() > 0 {
				evalREPLChunk(sess, buf.String(), out, errOut, cfg, running, interrupted)
			}
			if cfg.interactive {
				fmt.Fprintln(out)
			}
			return
		}

		trimmed := strings.TrimSpace(line)

		// Meta-commands only trigger when not inside a multi-line construct.
		if buf.Len() == 0 {
			if trimmed == "" {
				continue
			}
			if strings.HasPrefix(trimmed, ":") {
				done, reset := handleREPLCommand(eng, sess, trimmed, out, errOut, cfg.colors)
				if done {
					return
				}
				if reset {
					eng.Close()
					eng, sess = newSession()
					fmt.Fprintln(out, "session reset")
				}
				continue
			}
		}

		if buf.Len() > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(line)

		src := buf.String()
		if isIncompleteInput(src) {
			continue
		}
		buf.Reset()
		evalREPLChunk(sess, src, out, errOut, cfg, running, interrupted)
	}
}

func evalREPLChunk(
	sess *evie.Session,
	src string,
	out, errOut io.Writer,
	cfg replConfig,
	running, interrupted *atomic.Bool,
) {
	if interrupted != nil {
		interrupted.Store(false)
	}
	if running != nil {
		running.Store(true)
	}
	before := time.Now()
	val, hasVal, err := sess.Eval(src)
	elapsed := time.Since(before)
	if running != nil {
		running.Store(false)
	}
	if interrupted != nil {
		interrupted.Store(false)
	}

	if err != nil {
		fmt.Fprint(errOut, evie.Format(err, cfg.colors))
		return
	}
	if hasVal && !val.IsNil() {
		fmt.Fprintln(out, val)
	}
	if cfg.timing {
		fmt.Fprintf(out, "(%v)\n", elapsed.Round(time.Microsecond))
	}
}

func handleREPLCommand(eng *evie.Engine, sess *evie.Session, line string, out, errOut io.Writer, colors bool) (quit bool, reset bool) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return false, false
	}
	cmd := fields[0]
	rest := strings.TrimSpace(strings.TrimPrefix(line, cmd))

	switch cmd {
	case ":quit", ":exit", ":q":
		return true, false

	case ":help", ":h":
		fmt.Fprint(out, replHelp)

	case ":reset", ":clear":
		return false, true

	case ":dump":
		eng.Dump(out)

	case ":vars", ":env":
		symbols := sess.Symbols()
		if len(symbols) == 0 {
			fmt.Fprintln(out, "(no bindings)")
			return false, false
		}
		for _, b := range symbols {
			switch {
			case b.IsImport:
				fmt.Fprintf(out, "  import %s\n", b.Name)
			case b.Value.TypeOf() == "function":
				fmt.Fprintf(out, "  fn %s\n", b.Name)
			case b.Value.TypeOf() == "class":
				fmt.Fprintf(out, "  class %s\n", b.Name)
			case b.IsStatic:
				fmt.Fprintf(out, "  const %s = %s\n", b.Name, b.Value)
			default:
				fmt.Fprintf(out, "  var %s = %s\n", b.Name, b.Value)
			}
		}

	case ":import":
		importPath, alias, err := parseImportCommand(rest)
		if err != nil {
			fmt.Fprintf(errOut, "error: %v\n", err)
			return false, false
		}
		dep, err := sess.Import(importPath, alias)
		if err != nil {
			fmt.Fprint(errOut, evie.Format(err, colors))
			return false, false
		}
		bound := alias
		if bound == "" {
			bound = dep.Name()
		}
		fmt.Fprintf(out, "imported %s as %s\n", importPath, bound)

	case ":load":
		filePath := unquoteArg(rest)
		if filePath == "" {
			fmt.Fprintln(errOut, "usage: :load <file.ev>")
			return false, false
		}
		val, hasVal, err := sess.LoadFile(filePath)
		if err != nil {
			fmt.Fprint(errOut, evie.Format(err, colors))
			return false, false
		}
		if hasVal && !val.IsNil() {
			fmt.Fprintln(out, val)
		}

	default:
		fmt.Fprintf(errOut, "unknown command %q — type :help for a list of commands\n", cmd)
	}
	return false, false
}

// isIncompleteInput reports whether src appears to be an unfinished multi-line
// construct (an unclosed `{`, `(`, `[`, template string `` `...` ``, or a
// trailing `=>` arrow). Mismatched closing delimiters return false so the
// parser reports the syntax error immediately.
func isIncompleteInput(src string) bool {
	var stack []byte
	inTemplate := false
	var templateBraceDepths []int

	n := len(src)
	i := 0
	for i < n {
		c := src[i]

		if inTemplate {
			if c == '\\' && i+1 < n {
				i += 2
				continue
			}
			if c == '`' {
				inTemplate = false
				i++
				continue
			}
			if c == '{' {
				templateBraceDepths = append(templateBraceDepths, len(stack))
				stack = append(stack, '{')
				inTemplate = false
				i++
				continue
			}
			i++
			continue
		}

		// Line comment
		if c == '/' && i+1 < n && src[i+1] == '/' {
			for i < n && src[i] != '\n' {
				i++
			}
			continue
		}

		// Regular double-quoted string (single-line in Evie)
		if c == '"' {
			i++
			for i < n && src[i] != '"' && src[i] != '\n' {
				if src[i] == '\\' && i+1 < n {
					i += 2
					continue
				}
				i++
			}
			if i < n && src[i] == '"' {
				i++
			}
			continue
		}

		// Multi-line template literal
		if c == '`' {
			inTemplate = true
			i++
			continue
		}

		switch c {
		case '{', '(', '[':
			stack = append(stack, c)
		case '}':
			if len(stack) == 0 || stack[len(stack)-1] != '{' {
				return false
			}
			stack = stack[:len(stack)-1]
			if k := len(templateBraceDepths); k > 0 && templateBraceDepths[k-1] == len(stack) {
				templateBraceDepths = templateBraceDepths[:k-1]
				inTemplate = true
			}
		case ')':
			if len(stack) == 0 || stack[len(stack)-1] != '(' {
				return false
			}
			stack = stack[:len(stack)-1]
		case ']':
			if len(stack) == 0 || stack[len(stack)-1] != '[' {
				return false
			}
			stack = stack[:len(stack)-1]
		}
		i++
	}

	if inTemplate || len(stack) > 0 {
		return true
	}

	trimmed := strings.TrimSpace(stripTrailingComment(src))
	return strings.HasSuffix(trimmed, "=>")
}

func stripTrailingComment(src string) string {
	lastNL := strings.LastIndexByte(src, '\n')
	line := src[lastNL+1:]
	inStr := false
	for i := 0; i < len(line); i++ {
		if inStr {
			if line[i] == '\\' && i+1 < len(line) {
				i++
				continue
			}
			if line[i] == '"' {
				inStr = false
			}
			continue
		}
		if line[i] == '"' {
			inStr = true
			continue
		}
		if line[i] == '/' && i+1 < len(line) && line[i+1] == '/' {
			return src[:lastNL+1] + line[:i]
		}
	}
	return src
}

func parseImportCommand(rest string) (importPath, alias string, err error) {
	parts := strings.Fields(rest)
	switch len(parts) {
	case 1:
		return unquoteArg(parts[0]), "", nil
	case 3:
		if parts[1] != "as" {
			return "", "", fmt.Errorf("usage: :import <path> [as <alias>]")
		}
		return unquoteArg(parts[0]), parts[2], nil
	default:
		return "", "", fmt.Errorf("usage: :import <path> [as <alias>]")
	}
}

func unquoteArg(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		return s[1 : len(s)-1]
	}
	return s
}

func isCharDevice(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
