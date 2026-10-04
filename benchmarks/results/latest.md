# Evie benchmarks

`f251df4` on `main` · 2026-10-04 08:40 UTC · go1.27.2 · 16 CPUs

Baseline: `8314eee` (2026-10-01)

| benchmark | median | min | ± | alloc | Δ base |
|---|---:|---:|---:|---:|---:|
| lang/arrays | 26.2ms | 25.1ms | 2.7% | 7.0MB | +0.7% |
| lang/calls | 57.1ms | 54.8ms | 3.6% | 8.4MB | -4.6% |
| lang/closures | 56.3ms | 54.8ms | 2.8% | 33.6MB | +1.4% |
| lang/conditionals | 507.4ms | 501.9ms | 0.4% | 24KB | +0.9% |
| lang/errors | 64.9ms | 63.1ms | 3.9% | 36.1MB | -2.8% |
| lang/globals | 118.7ms | 117.9ms | 0.9% | 20KB | +0.3% |
| lang/higher_order | 91.9ms | 88.9ms | 2.1% | 31.0MB | +3.9% |
| lang/host_calls | 45.9ms | 44.2ms | 2.5% | 42KB | -3.5% |
| lang/inheritance | 37.3ms | 35.6ms | 3.4% | 34KB | -1.5% |
| lang/loops | 68.2ms | 66.8ms | 3.5% | 60KB | +4.3% |
| lang/method_calls | 92.1ms | 90.4ms | 1.6% | 47KB | -11.8% |
| lang/objects | 116.3ms | 114.6ms | 1.0% | 65.6MB | +1.0% |
| lang/strings | 80.8ms | 78.6ms | 2.6% | 61.5MB | -2.3% |
| lang/structs | 64.2ms | 62.8ms | 2.4% | 15.3MB | -2.1% |
| lang/switch | 142.3ms | 141.8ms | 0.8% | 30KB | -0.9% |
| lang/tasks | 92.2ms | 90.6ms | 1.6% | 9.3MB | -1.4% |
| math/ackermann | 67.3ms | 65.8ms | 1.1% | 385KB | -4.5% |
| math/collatz | 406.9ms | 404.0ms | 0.4% | 23KB | -2.6% |
| math/divisors | 274.6ms | 272.3ms | 0.6% | 25KB | -1.7% |
| math/fib | 95.7ms | 94.7ms | 1.1% | 18KB | +0.1% |
| math/fib_iter | 89.6ms | 88.0ms | 1.7% | 21KB | -10.3% |
| math/gcd | 75.6ms | 74.1ms | 1.8% | 22KB | -4.8% |
| math/gcd_rec | 82.2ms | 79.7ms | 1.2% | 21KB | -2.0% |
| math/leibniz | 95.5ms | 92.0ms | 2.4% | 18KB | -7.5% |
| math/mandelbrot | 69.0ms | 68.2ms | 1.0% | 28KB | -13.9% |
| math/matmul | 20.8ms | 19.0ms | 7.4% | 1.2MB | -9.6% |
| math/primes_trial | 339.7ms | 337.5ms | 0.7% | 22KB | -0.4% |
| math/sieve | 194.2ms | 190.0ms | 2.8% | 164.7MB | -2.3% |
| math/spectral_norm | 31.7ms | 29.6ms | 7.2% | 57KB | -6.7% |
| math/tak | 19.6ms | 18.7ms | 2.2% | 22KB | -3.2% |

Geomean median: **84.2ms** (-3.0% vs baseline)
