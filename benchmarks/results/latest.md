# Evie benchmarks

`60cfe96` on `main` · 2026-11-21 17:19 UTC · go1.27.2 · 16 CPUs

Baseline: `003cbaf` (2026-10-24)

| benchmark | median | min | ± | alloc | Δ base |
|---|---:|---:|---:|---:|---:|
| lang/arrays | 29.8ms | 27.0ms | 4.9% | 7.0MB | +13.7% |
| lang/calls | 65.1ms | 63.5ms | 3.0% | 8.4MB | +14.2% |
| lang/classes | 71.1ms | 69.0ms | 12.3% | 15.3MB | - |
| lang/closures | 62.0ms | 58.6ms | 11.0% | 36.7MB | +10.1% |
| lang/conditionals | 512.3ms | 501.3ms | 1.2% | 29KB | +1.0% |
| lang/errors | 72.0ms | 71.3ms | 3.6% | 36.1MB | +11.0% |
| lang/globals | 119.6ms | 118.6ms | 1.1% | 25KB | +0.7% |
| lang/higher_order | 92.1ms | 89.3ms | 3.4% | 31.0MB | +0.2% |
| lang/host_calls | 50.2ms | 47.0ms | 2.5% | 55KB | +9.4% |
| lang/inheritance | 42.0ms | 39.7ms | 2.4% | 40KB | +12.6% |
| lang/loops | 70.6ms | 68.1ms | 6.8% | 65KB | +3.5% |
| lang/method_calls | 104.1ms | 97.7ms | 8.5% | 53KB | +12.9% |
| lang/objects | 163.2ms | 130.4ms | 28.7% | 65.7MB | +40.3% |
| lang/strings | 103.1ms | 90.5ms | 13.7% | 61.5MB | +27.5% |
| lang/switch | 151.2ms | 143.0ms | 4.3% | 36KB | +6.2% |
| lang/tasks | 108.4ms | 106.4ms | 3.4% | 17.6MB | +17.6% |
| math/ackermann | 64.9ms | 62.2ms | 3.1% | 390KB | -3.6% |
| math/collatz | 419.1ms | 410.9ms | 4.1% | 28KB | +3.0% |
| math/divisors | 281.7ms | 274.1ms | 1.2% | 30KB | +2.6% |
| math/fib | 103.5ms | 98.4ms | 4.2% | 23KB | +8.0% |
| math/fib_iter | 111.9ms | 106.7ms | 3.9% | 26KB | +24.9% |
| math/gcd | 81.2ms | 77.8ms | 4.6% | 27KB | +7.4% |
| math/gcd_rec | 92.4ms | 86.6ms | 12.8% | 26KB | +12.3% |
| math/leibniz | 114.2ms | 105.3ms | 13.1% | 23KB | +19.7% |
| math/mandelbrot | 90.8ms | 82.8ms | 8.9% | 33KB | +31.6% |
| math/matmul | 27.0ms | 24.5ms | 6.1% | 1.2MB | +29.8% |
| math/primes_trial | 350.9ms | 343.0ms | 2.1% | 28KB | +3.3% |
| math/sieve | 230.8ms | 216.7ms | 3.6% | 164.7MB | +18.8% |
| math/spectral_norm | 35.9ms | 34.7ms | 4.7% | 63KB | +13.2% |
| math/tak | 22.5ms | 21.0ms | 3.3% | 27KB | +14.8% |

Geomean median: **94.5ms** (+12.2% vs baseline)
