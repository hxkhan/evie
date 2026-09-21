# Evie benchmarks

`08ca012` on `no-gil-neo` · 2026-09-21 13:50 UTC · go1.27.3 · 16 CPUs

| benchmark | median | min | ± | alloc | Δ base |
|---|---:|---:|---:|---:|---:|
| lang/arrays | 59.8ms | 56.6ms | 4.1% | 7.1MB | - |
| lang/calls | 87.5ms | 83.3ms | 7.0% | 10.7MB | - |
| lang/closures | 79.0ms | 74.2ms | 3.8% | 33.6MB | - |
| lang/conditionals | 754.5ms | 695.5ms | 6.6% | 16KB | - |
| lang/errors | 89.5ms | 78.2ms | 27.0% | 2.1MB | - |
| lang/globals | 142.7ms | 134.4ms | 29.1% | 14KB | - |
| lang/higher_order | 154.6ms | 102.0ms | 40.7% | 31.1MB | - |
| lang/inheritance | 71.5ms | 70.0ms | 2.9% | 36.6MB | - |
| lang/loops | 112.2ms | 75.4ms | 20.1% | 52KB | - |
| lang/objects | 240.6ms | 217.6ms | 7.5% | 50.4MB | - |
| lang/strings | 157.4ms | 124.8ms | 13.7% | 64.2MB | - |
| lang/structs | 209.7ms | 189.4ms | 4.6% | 39.7MB | - |
| lang/switch | 199.6ms | 192.6ms | 5.8% | 21KB | - |
| lang/synced | 68.5ms | 67.6ms | 4.2% | 22KB | - |
| lang/tasks | 56.5ms | 53.3ms | 7.3% | 8.9MB | - |
| math/ackermann | 210.4ms | 188.7ms | 9.6% | 85.2MB | - |
| math/collatz | 602.0ms | 539.7ms | 12.0% | 16KB | - |
| math/divisors | 429.7ms | 371.4ms | 6.2% | 17KB | - |
| math/fib | 157.5ms | 148.5ms | 3.1% | 12KB | - |
| math/fib_iter | 138.5ms | 136.7ms | 1.7% | 15KB | - |
| math/gcd | 125.2ms | 102.9ms | 17.1% | 15KB | - |
| math/gcd_rec | 174.1ms | 162.9ms | 4.8% | 15KB | - |
| math/leibniz | 245.4ms | 229.7ms | 4.1% | 12KB | - |
| math/mandelbrot | 111.9ms | 103.8ms | 9.5% | 19KB | - |
| math/matmul | 78.7ms | 73.4ms | 5.8% | 1.2MB | - |
| math/primes_trial | 456.3ms | 447.8ms | 6.8% | 15KB | - |
| math/sieve | 534.6ms | 469.3ms | 5.7% | 164.7MB | - |
| math/spectral_norm | 58.6ms | 55.0ms | 9.7% | 43KB | - |
| math/tak | 66.2ms | 66.0ms | 0.7% | 41.4MB | - |

Geomean median: **151.2ms**
