# The Evie Programming Language

Evie is a dynamically typed programming language written in Go. The goal is to provide Go developers with a blazingly fast embeddable scripting language that does *not* depend on CGO. 

Here is some example code
```php
fn pow(x, n) {
    if (n == 0) return 1
    
    return x * pow(x, n-1)
}

fn clamp(value, min, max) {
    if (value < min) 
        return min
    else if (value > max)
        return max

    return value
}

fn factorial(n) {
    if (n < 2) return 1

    return n * factorial(n-1)
}

fn main() {
    echo pow(2, 3)
    echo clamp(50, 0, 100)
    echo clamp(50, 75, 100)
    echo clamp(50, 0, 25)
    echo factorial(5)
}
```

Also a concurrency example below
```php
fn print(message, duration) {
    await time.timer(duration)
    echo message
}

fn main() {
    go print("one", 100)
    go print("two", 200)
    go print("three", 300)
    go print("four", 400)
    go print("five", 500)
    go print("six", 600)
    go print("seven", 700)
    go print("eight", 800)
    go print("nine", 900)
    go print("ten", 1000)
}
```
To test this exact program, run `go run ./cli -t ./examples/go.ev`. Then remove all of the `go` keywords infront of the `print` calls and re-run to see the difference.

## Flags
- flag `-t` prints the execution time
- flag `-inline=true/false` enables or disables inlining certain instruction combinations for performance. It is `true` by default

## Goals
- Highly performant ✅
- Builtin concurrency ⏸️
- Very similar to Go and other existing languages ✅
- Embeddable in your Go applications ✅
- Can also be run in standalone mode for scripts ✅
- Builtin package manager ❌
- No cgo, written in pure Go ✅

> This project is in its early state. Many features are half baked.

## Benchmarks
| Language | fib(35)  | Host Language |
| :-       | :-       | :-            |
| [**Evie**](https://github.com/hxkhan/evie) | `478ms` | Go |
| [Lua 5.4.2](https://lua.org/) | `536ms` | C | 
| [QuickJS](https://bellard.org/quickjs/) | `703ms` | C | 
| [Python 3.13](https://python.org/) | `826ms` | C |
| [Wren](https://wren.io/) | `893ms` | C |
| [Tengo](https://github.com/d5/tengo) | `1603ms` | Go |

> These benchmarks were ran on an Intel i5-13400F. Each language 10 times and the average was taken.

All of these exist in the `examples` directory. To build the evie cli you can do:
```
git clone https://github.com/hxkhan/evie.git
cd evie
go build ./cli
time ./cli ./examples/fib.ev
```

To benchmark the other languages, you can grab your own versions from their respective websites. For example; if you have python installed then just do `time python ./examples/fib.py`, you might have to change `python` for `python3`.

> Keep in mind, `time` exists only on linux, use `Measure-Command` on Windows powershell like `Measure-Command { python ./examples/fib.py }`