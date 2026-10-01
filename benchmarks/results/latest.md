# Evie benchmarks

`bf4625f` on `gamma` · 2026-10-01 14:50 UTC · go1.27.2 · 16 CPUs

Baseline: `a0ab83c` (2026-09-24)

| benchmark | median | min | ± | alloc | Δ base |
|---|---:|---:|---:|---:|---:|
| lang/arrays | 26.1ms | 25.2ms | 2.1% | 7.0MB | -61.0% |
| lang/calls | 59.8ms | 58.8ms | 1.1% | 8.4MB | -38.2% |
| lang/closures | 55.5ms | 54.0ms | 3.5% | 33.6MB | -28.1% |
| lang/conditionals | 503.1ms | 499.2ms | 0.9% | 24KB | -25.3% |
| lang/errors | 66.7ms | 65.3ms | 3.3% | 36.1MB | -47.4% |
| lang/globals | 118.4ms | 116.9ms | 1.2% | 20KB | -46.2% |
| lang/higher_order | 88.4ms | 86.8ms | 2.5% | 31.0MB | -49.9% |
| lang/host_calls | 47.6ms | 46.9ms | 1.3% | 6.1MB | - |
| lang/inheritance | 37.8ms | 36.5ms | 1.6% | 34KB | -51.4% |
| lang/loops | 65.4ms | 64.7ms | 1.4% | 60KB | -23.2% |
| lang/method_calls | 104.4ms | 103.4ms | 1.2% | 12.3MB | - |
| lang/objects | 115.1ms | 112.8ms | 1.5% | 65.6MB | -29.2% |
| lang/strings | 82.8ms | 81.1ms | 3.0% | 63.0MB | -26.8% |
| lang/structs | 65.6ms | 63.9ms | 2.0% | 15.3MB | -68.3% |
| lang/switch | 143.6ms | 141.2ms | 0.9% | 30KB | -27.5% |
| lang/tasks | 93.5ms | 91.5ms | 1.6% | 9.2MB | +34.6% |
| math/ackermann | 70.5ms | 66.6ms | 2.6% | 385KB | -77.4% |
| math/collatz | 417.8ms | 411.4ms | 0.8% | 23KB | -44.2% |
| math/divisors | 279.4ms | 273.9ms | 0.8% | 25KB | -29.3% |
| math/fib | 95.6ms | 94.0ms | 2.3% | 18KB | -52.2% |
| math/fib_iter | 99.8ms | 97.7ms | 1.0% | 21KB | -22.2% |
| math/gcd | 79.4ms | 77.8ms | 1.9% | 22KB | -25.1% |
| math/gcd_rec | 83.9ms | 83.0ms | 2.4% | 21KB | -53.7% |
| math/leibniz | 103.2ms | 101.7ms | 1.3% | 18KB | -35.5% |
| math/mandelbrot | 80.1ms | 78.8ms | 0.9% | 27KB | -23.1% |
| math/matmul | 23.0ms | 22.5ms | 2.9% | 1.2MB | -77.1% |
| math/primes_trial | 341.1ms | 337.7ms | 0.5% | 22KB | -35.8% |
| math/sieve | 198.8ms | 193.8ms | 2.4% | 164.7MB | -60.7% |
| math/spectral_norm | 34.0ms | 32.8ms | 3.3% | 57KB | -44.2% |
| math/tak | 20.2ms | 19.4ms | 1.9% | 22KB | -70.3% |

Geomean median: **86.9ms** (-44.8% vs baseline)
