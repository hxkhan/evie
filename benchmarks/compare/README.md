# Cross-Language Benchmarks

Compares Evie against other dynamically typed scripting interpreters on a shared suite of 8 workloads. Every implementation must print the exact checksum in `suites/<name>/expect.txt`, so a broken port fails instead of reporting a misleading time.

## Runtimes

The runner builds a fresh `evie` binary from this repository and automatically picks up whichever of the following runtimes are installed on `PATH`:

| Name | Binary | Notes |
|---|---|---|
| `evie` | built from `./evie` | Pure Go interpreter |
| `lua54` | `lua54` or `lua` | PUC Lua 5.4 |
| `qjs` | `qjs` | QuickJS |
| `v8-jitless` | `node --jitless` | V8 Ignition interpreter (no JIT) |
| `python` | `python` or `python3` | CPython 3 |
| `wren` | `wren` | Wren VM |
| `tengo` | `tengo` | Pure Go interpreter |
| `node-jit` | `node` | Opt-in via `-jit` or `-langs` (V8 TurboFan JIT) |

## Usage

```bash
# Run all 8 benchmarks across all installed interpreters (5 runs + 1 warmup each)
go run ./benchmarks/compare

# Run a subset of benchmarks or runtimes
go run ./benchmarks/compare -filter "fib|sieve" -n 5
go run ./benchmarks/compare -langs evie,lua54,qjs,v8-jitless,wren,tengo

# Include Node.js with V8 JIT enabled
go run ./benchmarks/compare -jit

# Export a Markdown table or JSON report
go run ./benchmarks/compare -md benchmarks/compare/results.md
```

On Windows, [bench.ps1](./bench.ps1) wraps the runner and supports pinning P-cores with `-Affinity` (for example `-Affinity 0x0FFF` on 6P+4E Intel CPUs):

```powershell
.\benchmarks\compare\bench.ps1 -Runs 5 -Affinity 0x0FFF
```

## Workloads

| Benchmark | Stresses |
|---|---|
| `fib` | Recursive function calls (`fib(35)`, 29.86M calls) |
| `sieve` | 1M-element array growth and tight indexed reads/writes |
| `mandelbrot` | Floating-point arithmetic, nested loops, early `break` |
| `matmul` | 120×120 2D nested array indexing and triple-nested loops |
| `closures` | Closure creation, captured mutable state, 1M+ indirect calls |
| `objects` | Dynamic string-keyed hash maps (600k insert/update/read ops) |
| `classes` | OOP instantiation, fields, sibling method calls, polymorphic dispatch |
| `strings` | Concatenation, `split`, `join`, `replace`, `contains`, allocation/GC |
