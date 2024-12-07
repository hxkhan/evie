# The Evie Language

Evie is a dynamically typed scripting language written in Golang.

Here is some example code
```go
package main

fn main() {
    echo fib(35)
}

fn fib(n) {
    if (n < 2) return n
    
    return fib(n-1) + fib(n-2)
}
```

## Features
- Highly performant
- Very similar to Go and other existing languages
- Embeddable in your Go applications
- Can also be run in standalone mode for scripts
- No cgo, written in pure Go

## Benchmarks
| | fib(35)  |
| :--- |    ---: |
| [**Evie**](https://github.com/hxkhan/evie) | `692ms` |
| [Tengo](https://github.com/d5/tengo) | `1,533ms` |