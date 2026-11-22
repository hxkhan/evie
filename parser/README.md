# Evie Parser

The `parser` package implements a hand-written **recursive-descent parser with precedence climbing** that converts tokens from the internal lexer (`internal/lexer`) into an `ast.Node` tree.

## Entry Points

- `parser.Parse(input []byte) (ast.Node, error)` — parses an anonymous script.
- `parser.ParseSource(src *diag.Source) (ast.Node, error)` — parses a named source; syntax errors carry full `diag.Report` spans and notes pointing at the source text.
- `parser.PeekPackageName(src *diag.Source) string` — scans only the leading `package <name>` header without parsing the full file (used by the multi-file project loader).

## Architecture

Every top-level file with a `package` header parses into an `ast.Package`; bare scripts parse into an `ast.Block`.

- **Core driver**: `parse(precedenceLevel int, asExpr bool) ast.Node` calls `parseNode`, which parses one primary, prefix or keyword construct, then `parseInfixExpression`, which climbs postfix forms and binary operators for as long as the next operator meets `precedenceLevel`.
- **Lists**: every comma-separated list (imports, parameters, arguments, objects, arrays) goes through `ps.list`, which allows a trailing comma.
- **Line breaks**: a `(` or `[` that starts a line begins a new statement instead of calling or indexing the line before; `.` still continues it.
- **Statement vs. expression boundary**: statements (`if`, `while`, `for`, `switch`, declarations, assignments) are rejected in value-expression positions (`mustBeValue`), while arrow bodies permit assignments and `echo`.
- **Recursion guard**: nested constructs and `else if` chains track recursion depth against `maxParseDepth` (256), returning a clean syntax error on deeply nested pathological input instead of overflowing the Go stack. The lexer, which reads template literals ahead of the parser, stops at a deeper nesting of its own (`maxTemplateDepth`), so the parser's error is the one reported.
- **Panic-free boundary**: internal syntax errors abort parsing via `fail(...)` and are caught by `ParseSource`'s `defer recover()`, returning `*parser.Error` (which embeds `diag.Report` with source excerpts and opening-delimiter notes).

## Operator Precedence

From lowest (`0`) to highest (`5`):

| Level | Operators |
| :---: | :--- |
| `0` | `\|\|` |
| `1` | `&&` |
| `2` | `==`, `!=`, `<`, `<=`, `>`, `>=`, `is` |
| `3` | `+`, `-` |
| `4` | `*`, `/`, `%` |
| `5` | Unary `+`, `-`, `!`, `await`, `go` |
| Postfix | Calls `f(...)`, member access `x.y`, existence `x.y?`, indexing `x[i]`, slicing `x[a:b]` |
