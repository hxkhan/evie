# Evie benchmarks

`76b718e` on `main` · 2026-11-22 21:43 UTC · go1.27.2 · 16 CPUs

Baseline: `e6bb6ce` (2026-11-22)

| benchmark | median | min | ± | alloc | Δ base |
|---|---:|---:|---:|---:|---:|
| lang/arrays | 11.5ms | 9.9ms | 8.2% | 7.0MB | +7.5% |
| lang/calls | 40.8ms | 38.4ms | 4.4% | 8.4MB | -7.1% |
| lang/classes | 32.8ms | 31.4ms | 3.1% | 12.3MB | -3.7% |
| lang/closures | 51.6ms | 50.1ms | 3.6% | 33.6MB | -3.1% |
| lang/conditionals | 67.3ms | 65.8ms | 4.7% | 31KB | -21.7% |
| lang/errors | 44.2ms | 41.7ms | 4.2% | 36.1MB | +9.9% |
| lang/globals | 19.0ms | 18.1ms | 3.8% | 27KB | -6.8% |
| lang/higher_order | 39.8ms | 37.9ms | 9.4% | 31.0MB | +7.3% |
| lang/host_calls | 24.2ms | 23.1ms | 3.0% | 47KB | -1.5% |
| lang/inheritance | 25.8ms | 23.6ms | 5.4% | 44KB | -2.3% |
| lang/loops | 19.6ms | 19.5ms | 3.9% | 70KB | -14.1% |
| lang/method_calls | 45.0ms | 44.3ms | 2.5% | 58KB | -3.7% |
| lang/objects | 35.1ms | 34.1ms | 4.9% | 21.4MB | -56.7% |
| lang/strings | 67.6ms | 59.2ms | 15.6% | 58.5MB | -25.9% |
| lang/switch | 42.9ms | 40.2ms | 5.8% | 38KB | -13.9% |
| lang/tasks | 95.1ms | 88.5ms | 2.7% | 17.6MB | -18.7% |
| math/ackermann | 52.3ms | 49.0ms | 3.6% | 277KB | -22.9% |
| math/collatz | 101.2ms | 100.5ms | 1.8% | 30KB | -33.7% |
| math/divisors | 54.6ms | 53.6ms | 3.3% | 32KB | -39.6% |
| math/fib | 82.3ms | 78.5ms | 7.9% | 24KB | -30.7% |
| math/fib_iter | 67.9ms | 65.6ms | 2.2% | 28KB | -38.8% |
| math/gcd | 40.4ms | 39.3ms | 2.0% | 29KB | -35.4% |
| math/gcd_rec | 44.3ms | 42.7ms | 3.0% | 28KB | -35.5% |
| math/leibniz | 77.6ms | 75.2ms | 4.0% | 25KB | -48.7% |
| math/mandelbrot | 50.3ms | 48.3ms | 2.6% | 35KB | -48.2% |
| math/matmul | 12.4ms | 11.0ms | 5.8% | 1.2MB | -53.9% |
| math/primes_trial | 59.3ms | 56.3ms | 3.3% | 30KB | -55.5% |
| math/sieve | 98.8ms | 92.9ms | 5.9% | 64.0MB | -67.7% |
| math/spectral_norm | 22.0ms | 20.7ms | 3.2% | 66KB | -48.8% |
| math/tak | 15.8ms | 14.5ms | 4.4% | 26KB | -45.7% |

Geomean median: **41.2ms** (-28.8% vs baseline)
