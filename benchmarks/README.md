# Evie benchmarks

A suite of Evie programs for tracking interpreter performance over time. Every benchmark is a normal `.ev` program whose `main()` returns a checksum; the runner checks that checksum against the file's `// expect:` header, so a benchmark that gets faster by computing the wrong answer fails instead of looking like a win.

## Running

```bash
go run ./benchmarks                     # run everything, compare against results/baseline.json
go run ./benchmarks -filter math/ -n 10 # only paths matching the regexp, 10 timed runs each
go run ./benchmarks -save               # also store the run in results/<date>_<commit>.json
go run ./benchmarks -save-baseline      # make this run the new baseline
go run ./benchmarks -md report.md       # also write a markdown report
```

Each benchmark gets one untimed warm-up run (`-warmup`) followed by `-n` timed runs (default 5). Every run uses a fresh `evie.Loader`, so timings include parsing and compiling, which are negligible next to execution. The table shows the median, the minimum, the relative standard deviation, the bytes allocated during one run, and the change against the baseline. The last line is the geometric mean of all medians and the geomean change against the baseline. The command exits with status 1 if any benchmark fails.

> On this Windows machine, Smart App Control blocks binaries built under `%TEMP%`. Set `GOTMPDIR` to an absolute path outside it, for example `GOTMPDIR=C:/Users/hxkhan/Documents/Projects/.evie-gotmp go run ./benchmarks`.

## Suite

### `math/`: numeric hot loops

| benchmark | stresses |
|---|---|
| `fib` | recursive calls, comparison, addition |
| `fib_iter` | tight `while` loop over mutable locals |
| `collatz` | branches, `%`, `/`, long inner loops |
| `gcd` / `gcd_rec` | Euclid's algorithm, iterative vs recursive |
| `primes_trial` | trial division, nested loops, early `return` |
| `sieve` | 2M-element array growth, indexed reads and writes |
| `mandelbrot` | float multiply and add, `break` |
| `leibniz` | float division and accumulation |
| `spectral_norm` | float math through array indexing and small calls |
| `matmul` | nested arrays, `a[i][j]` access |
| `divisors` | divisor sums, `%`, `/` |
| `tak` | three-argument recursion dominated by calls |
| `ackermann` | very deep recursion (stresses the fiber stack and box pool) |

### `lang/`: language features

| benchmark | stresses |
|---|---|
| `calls` | static calls with 0–4 args, calls through locals, lambdas, variadics |
| `closures` | closure creation, captured reads and writes, nested captures |
| `loops` | `for ... in` with and without an index, `break`/`continue`, nested `while` |
| `conditionals` | `if`/`else if` chains, `&&`/`\|\|`/`!`, `is` |
| `switch` | value, multi-value, tagless and string `switch` |
| `arrays` | push/pop churn, indexed read-modify-write, `slice`/`reverse`/`contains` |
| `higher_order` | builtin `filter`/`map` calling back into Evie |
| `objects` | object literals, field get/set, dynamic keys, `has`/`keys`, iteration |
| `strings` | concatenation, templates, `split`/`join`/`toUpper`, iteration |
| `structs` | instantiation, methods, `this` field access, bound methods |
| `inheritance` | polymorphic dispatch through a three-level hierarchy |
| `errors` | `throw` through several frames and `catch` |
| `globals` | package constants and mutable `var` globals |
| `tasks` | `go` spawn/await overhead, `await.all` fan-out, parallel CPU work |
| `synced` | `synced` functions and contended `synced` blocks |

## Adding a benchmark

1. Drop a `.ev` file into `math/` or `lang/` (or a new subfolder). Start it with a comment saying what it stresses, followed by `// expect: <result>`.
2. Make `main()` return a deterministic value that depends on all the work, so the work can't be skipped.
3. Verify the expected value independently, for example with an equivalent Go program, instead of copying Evie's output.
4. Aim for 50–700ms per run so that timer noise stays small and the whole suite stays quick.
5. Once you change a benchmark, its results can no longer be compared with older runs, so save a new baseline in the same commit.

## Results

`results/latest.md` is a readable report of the most recent recorded run (`-md benchmarks/results/latest.md`). `results/baseline.json` is the reference that runs are compared against. Runs saved with `-save` stay alongside it as a history. Each file records the commit, the branch, whether the tree was dirty, the Go version, the CPU count and the per-benchmark numbers. Compare a run against any older one with `-baseline results/<file>.json`.
