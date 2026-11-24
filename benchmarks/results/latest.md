# Evie benchmarks

`1e084d2` on `main` · 2026-11-24 17:26 UTC · go1.27.2 · 16 CPUs

Baseline: `76b718e` (2026-11-22)

| benchmark | median | min | ± | alloc | Δ base |
|---|---:|---:|---:|---:|---:|
| lang/arrays | 10.5ms | 9.5ms | 3.9% | 7.1MB | -8.7% |
| lang/calls | 42.0ms | 38.5ms | 3.7% | 8.4MB | +2.9% |
| lang/classes | 32.2ms | 30.6ms | 3.2% | 12.3MB | -1.7% |
| lang/closures | 47.8ms | 46.7ms | 3.6% | 33.6MB | -7.5% |
| lang/conditionals | 67.2ms | 65.6ms | 2.1% | 31KB | -0.1% |
| lang/errors | 39.8ms | 38.0ms | 6.8% | 39.1MB | -9.8% |
| lang/globals | 20.0ms | 19.0ms | 4.8% | 27KB | +5.2% |
| lang/higher_order | 36.1ms | 33.5ms | 7.6% | 31.0MB | -9.1% |
| lang/host_calls | 24.1ms | 22.9ms | 5.1% | 53KB | -0.7% |
| lang/inheritance | 25.4ms | 24.1ms | 4.5% | 47KB | -1.6% |
| lang/loops | 21.0ms | 19.0ms | 9.9% | 69KB | +7.2% |
| lang/method_calls | 44.6ms | 42.0ms | 3.4% | 62KB | -1.0% |
| lang/objects | 35.4ms | 34.1ms | 3.4% | 21.4MB | +0.8% |
| lang/strings | 62.1ms | 58.7ms | 10.6% | 58.5MB | -8.1% |
| lang/switch | 41.8ms | 40.5ms | 1.6% | 39KB | -2.5% |
| lang/tasks | 91.4ms | 89.3ms | 2.1% | 17.5MB | -4.0% |
| math/ackermann | 49.6ms | 47.3ms | 3.0% | 277KB | -5.1% |
| math/collatz | 99.4ms | 98.1ms | 1.0% | 30KB | -1.7% |
| math/divisors | 54.6ms | 53.5ms | 1.6% | 32KB | +0.0% |
| math/fib | 78.6ms | 76.6ms | 2.4% | 24KB | -4.5% |
| math/fib_iter | 68.6ms | 66.5ms | 2.7% | 27KB | +1.1% |
| math/gcd | 40.0ms | 38.0ms | 3.7% | 28KB | -1.0% |
| math/gcd_rec | 42.6ms | 40.5ms | 4.0% | 27KB | -3.9% |
| math/leibniz | 77.1ms | 74.6ms | 3.0% | 24KB | -0.7% |
| math/mandelbrot | 48.2ms | 47.1ms | 11.2% | 34KB | -4.3% |
| math/matmul | 12.1ms | 11.5ms | 3.6% | 1.2MB | -2.3% |
| math/primes_trial | 58.0ms | 56.9ms | 2.2% | 29KB | -2.1% |
| math/sieve | 102.0ms | 99.1ms | 3.0% | 64.0MB | +3.3% |
| math/spectral_norm | 21.0ms | 20.0ms | 3.5% | 68KB | -4.6% |
| math/tak | 14.9ms | 14.2ms | 4.9% | 26KB | -5.7% |

Geomean median: **40.2ms** (-2.4% vs baseline)
