# Evie Language Syntax

The sytax of this programming language is designed to be very familiar to existing developers. We don't do anything revolutionary. I think of this language as if Golang, Rust and JavaScript had a baby. Evie's design not only feels natural but also allows for plentiful optimization opportunities. I wanted to make the language *fast*, first of all. And so I had to find ways to make that easy for myself but at the same time not sacrifice on the flexibility of the language.

## Package
A package is simply a named container for a set of global bindings.
```go
package math

pi := 3.14
```
In this case, `math` is a package that contains a binding called `pi` with the value `3.14`.

```go
package main imports("io")

fn main() {
    io.println("Hello World")
}
```

And here we create a `main` package that imports an `io` package for printing purposes.

## Bindings
Now you might be wondering *"what are bindings"*? They're simply variables but I prefer to call them *a binding* because if you write `x := 10` then `x` is actually a constant and cannot be reassigned. It's the equivalent of `const x = 10` in JavaScript. If you want to create a binding that can *vary*, then you create a variable binding by adding `var` to the start. Like `var x := 10`.

### Why This Makes Sense
1. `:=` Always means *new binding*
    ```go
    name := "John"    // new immutable binding
    var score := 0    // new reassignable binding
    ```
    I really liked the `:=` operator from Go and I wanted to bring it to Evie but at the same time I liked the idea of explicit `mut` from Rust and wanted that safety by default.

2. `=` always means *reassignment*
    ```go
    score = score + 1 // works becuase 'score' was declared with 'var'
    name = "Jane"     // error because 'name' is not reassignable
    ```

Let's look at some JavaScript code
```js
// What developers SHOULD write:
const name = "John"
const items = []
let counter = 0

// What developers ACTUALLY write:
var name = "John"
var items = []
var counter = 0
```
The problem is most JS devs actually never end up using `const` even tho tutorials reiterate it again and again. Why? Because it's just easier to type out `var` and be done with it.

With Evie, we get the following
```go
// What developers will naturally write (taking the path of least resistance):
items := []              // Gets immutability by default! 🎉
name := "John"           // Safe by default!

// When they explicitly need mutability:
var counter := 0         // Intentional choice
```

## Functions
Functions on the package level are constants.
```rs
fn inc(n) {
    return n + 1
}

// -- OR --

inc := fn(n) {
    return n + 1
}
```
These two are basically identical. If you want a *varying* binding that starts off as a function then declare it like

```js
var inc := fn() {
    return n + 1
}
```

## Control flow
Control flow works exactly the same as Go.

### Conditional IF
```go
if x < 2 {
    io.println("yes")
}
```
You can also add as many `else if` as you want and optionally end it with an `else`.

Keep in mind, Evie requires blocks just like Go. This means you cannot write
```js
if (x < 2) io.println("yes")
```

### While Loop
We do have while loops whereas Go just uses `for`
```js
while x > 0 {
    io.println("ran")
    x -= 1
}
```
And `continue` and `break` works like usual.