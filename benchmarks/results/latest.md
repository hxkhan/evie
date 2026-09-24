# Evie benchmarks

`a0ab83c` on `no-gil-neo` · 2026-09-24 13:50 UTC · go1.27.3 · 16 CPUs

Baseline: `08ca012` (2026-09-21)

| benchmark | median | min | ± | alloc | Δ base |
|---|---:|---:|---:|---:|---:|
| lang/arrays | 66.9ms | 55.5ms | 28.3% | 7.1MB | +11.9% |
| lang/calls | 96.8ms | 80.4ms | 10.9% | 10.7MB | +10.6% |
| lang/closures | 77.2ms | 74.1ms | 7.7% | 33.6MB | -2.2% |
| lang/conditionals | 673.8ms | 662.5ms | 18.5% | 16KB | -10.7% |
| lang/errors | 126.9ms | 117.2ms | 4.9% | 2.1MB | +41.8% |
| lang/globals | 220.2ms | 197.1ms | 4.8% | 14KB | +54.3% |
| lang/higher_order | 176.5ms | 156.4ms | 18.5% | 31.1MB | +14.2% |
| lang/inheritance | 77.9ms | 74.3ms | 7.1% | 36.6MB | +8.9% |
| lang/loops | 85.1ms | 82.8ms | 7.1% | 52KB | -24.1% |
| lang/objects | 162.5ms | 157.0ms | 2.5% | 50.4MB | -32.4% |
| lang/strings | 113.1ms | 100.4ms | 15.5% | 64.2MB | -28.1% |
| lang/structs | 206.7ms | 156.6ms | 7.9% | 39.7MB | -1.4% |
| lang/switch | 198.0ms | 192.0ms | 12.3% | 21KB | -0.8% |
| lang/synced | 88.3ms | 81.7ms | 5.5% | 22KB | +29.0% |
| lang/tasks | 69.5ms | 66.9ms | 13.4% | 8.9MB | +23.0% |
| math/ackermann | 312.3ms | 275.4ms | 7.1% | 85.2MB | +48.4% |
| math/collatz | 748.2ms | 593.5ms | 10.5% | 16KB | +24.3% |
| math/divisors | 395.2ms | 357.1ms | 14.6% | 17KB | -8.0% |
| math/fib | 200.0ms | 149.0ms | 11.9% | 12KB | +27.0% |
| math/fib_iter | 128.3ms | 125.7ms | 1.8% | 15KB | -7.3% |
| math/gcd | 106.0ms | 99.1ms | 9.8% | 15KB | -15.4% |
| math/gcd_rec | 181.4ms | 149.5ms | 8.2% | 15KB | +4.2% |
| math/leibniz | 160.1ms | 154.4ms | 7.9% | 12KB | -34.8% |
| math/mandelbrot | 104.2ms | 101.0ms | 2.5% | 19KB | -7.0% |
| math/matmul | 100.5ms | 91.3ms | 3.8% | 1.2MB | +27.7% |
| math/primes_trial | 531.1ms | 447.3ms | 11.3% | 16KB | +16.4% |
| math/sieve | 505.4ms | 481.7ms | 7.7% | 164.7MB | -5.4% |
| math/spectral_norm | 60.9ms | 53.1ms | 17.2% | 43KB | +4.0% |
| math/tak | 68.0ms | 62.5ms | 9.1% | 41.4MB | +2.7% |

Geomean median: **156.4ms** (+3.5% vs baseline)
