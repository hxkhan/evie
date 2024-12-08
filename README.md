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

Concurrency example
```go
package main

fn print(message, duration) {
    await time.timer(duration) // timer will return a 'task' that we await on
    echo message
}

fn main() {
    // keyword 'go' starts a new coroutine
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
To test this exact program cd to `./use` and run `go run . -t go.es`
- flag `-t` prints the time it took to execute
- flag `-p` prints the *byte code* of the program, before running it
- flag `-o=true/false` enables or disables specialised instruction optimisations. It's `true` by default

## Features
- Highly performant
- Builtin concurrency
- Very similar to Go and other existing languages
- Embeddable in your Go applications
- Can also be run in standalone mode for scripts
- No cgo, written in pure Go

## Benchmarks
| | fib(35)  |
| :--- |    ---: |
| [**Evie**](https://github.com/hxkhan/evie) | `692ms` |
| [Tengo](https://github.com/d5/tengo) | `1533ms` |