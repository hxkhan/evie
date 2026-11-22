# Evie benchmarks

`e6bb6ce` on `main` · 2026-11-22 11:35 UTC · go1.27.2 · 16 CPUs

Baseline: `60cfe96` (2026-11-21)

| benchmark | median | min | ± | alloc | Δ base |
|---|---:|---:|---:|---:|---:|
| lang/arrays | 10.7ms | 10.1ms | 3.3% | 7.0MB | -64.2% |
| lang/calls | 43.9ms | 43.1ms | 3.2% | 8.4MB | -32.6% |
| lang/classes | 34.0ms | 32.7ms | 27.3% | 12.3MB | -52.1% |
| lang/closures | 53.3ms | 50.4ms | 5.0% | 33.6MB | -14.0% |
| lang/conditionals | 85.9ms | 83.4ms | 3.1% | 29KB | -83.2% |
| lang/errors | 40.2ms | 39.1ms | 3.0% | 36.1MB | -44.2% |
| lang/globals | 20.4ms | 20.1ms | 1.6% | 25KB | -82.9% |
| lang/higher_order | 37.1ms | 34.2ms | 5.8% | 31.0MB | -59.7% |
| lang/host_calls | 24.6ms | 24.0ms | 6.7% | 45KB | -51.0% |
| lang/inheritance | 26.5ms | 25.4ms | 3.6% | 41KB | -37.0% |
| lang/loops | 22.8ms | 22.3ms | 5.1% | 66KB | -67.6% |
| lang/method_calls | 46.8ms | 45.3ms | 11.4% | 56KB | -55.1% |
| lang/objects | 81.0ms | 73.1ms | 6.1% | 65.7MB | -50.3% |
| lang/strings | 91.2ms | 88.0ms | 7.2% | 61.5MB | -11.5% |
| lang/switch | 49.9ms | 47.4ms | 7.1% | 36KB | -67.0% |
| lang/tasks | 117.1ms | 104.1ms | 5.9% | 17.6MB | +8.0% |
| math/ackermann | 67.7ms | 65.0ms | 4.0% | 276KB | +4.4% |
| math/collatz | 152.6ms | 142.1ms | 6.4% | 28KB | -63.6% |
| math/divisors | 90.3ms | 82.6ms | 4.9% | 30KB | -67.9% |
| math/fib | 118.8ms | 101.1ms | 7.5% | 23KB | +14.8% |
| math/fib_iter | 110.9ms | 105.1ms | 4.7% | 26KB | -0.9% |
| math/gcd | 62.6ms | 61.5ms | 7.4% | 27KB | -22.8% |
| math/gcd_rec | 68.6ms | 65.9ms | 3.5% | 26KB | -25.7% |
| math/leibniz | 151.2ms | 136.4ms | 5.0% | 23KB | +32.4% |
| math/mandelbrot | 97.1ms | 89.6ms | 12.6% | 33KB | +7.0% |
| math/matmul | 26.8ms | 26.3ms | 45.8% | 1.2MB | -0.8% |
| math/primes_trial | 133.2ms | 111.9ms | 9.7% | 28KB | -62.0% |
| math/sieve | 305.7ms | 257.4ms | 13.9% | 164.7MB | +32.4% |
| math/spectral_norm | 43.0ms | 41.5ms | 20.9% | 64KB | +19.7% |
| math/tak | 29.1ms | 26.0ms | 13.1% | 25KB | +29.3% |

Geomean median: **57.8ms** (-38.9% vs baseline)
