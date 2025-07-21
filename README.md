# The Evie Programming Language

Evie is a dynamically typed scripting language written in Golang.

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
    await time.timer(duration) // timer will return a 'task' that we await on
    echo message
}

fn main() {
    // keyword 'go' starts a new fiber
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
To test this exact program, `cd` to `cli` and run `go run . -t go.es`
- flag `-t` prints the execution time
- flag `-o=true/false` enables or disables specialised instruction optimisations. It is `true` by default

## Features
- Highly performant
- Builtin concurrency
- Very similar to Go and other existing languages
- Embeddable in your Go applications
- Can also be run in standalone mode for scripts
- No cgo, written in pure Go

## Benchmarks
| Language | fib(35)  |
| :--- |    ---: |
| [**Evie**](https://github.com/hxkhan/evie) | `480ms` |
| [Tengo](https://github.com/d5/tengo) | `1560ms` |