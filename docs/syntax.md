# Evie Language Syntax

The syntax of Evie is designed to feel familiar, clean, and expressive. Think of it as the sweet spot where **Go**, **Rust**, and **JavaScript** meet:
- The clean simplicity, package structure, and `:=` binding style of **Go**.
- Safe-by-default immutability inspired by **Rust** (`var` is required to allow mutation).
- The dynamic expressiveness, first-class functions, closures, and object ergonomics of **JavaScript/TypeScript**.

---

## 1. Packages & Imports

Every standalone Evie file or module begins with a `package` declaration, optionally importing other packages.

```go
package main imports("io", "time")

fn main() {
    io.println("Hello from Evie!")
}
```

- Standard library packages (`io`, `fs`, `time`, `strings`, `lists`) can be imported directly.
- Package-level functions and bindings are namespaced under their package name when imported (`io.println`, `time.timer`).

---

## 2. Bindings & Mutability

Evie uses two distinct operators for declaration vs reassignment:
- `:=` always creates a **new binding**.
- `=` always **reassigns** an existing mutable binding.

### Safe by Default (Immutable Bindings)
When you declare a binding with `:=` alone, it is **immutable** (like `const` in JavaScript or `let` in Rust):

```go
name := "John"       // immutable binding
items := [1, 2, 3]   // immutable binding

name = "Jane"        // Compile Error: Assignment to constant binding
```

### Explicit Mutability (`var`)
To make a binding reassignable, prefix it with `var`:

```go
var counter := 0     // mutable binding
counter = counter + 1 // OK
counter += 5          // OK (compound assignment)
```

### Why This Design?
Most developers default to the shortest keyword (e.g. `var` in JS). In Evie, the path of least resistance (`x := 10`) gives you safety and immutability by default. When mutability is needed, `var x := 10` is an intentional, visible choice.

---

## 3. Data Types & Literals

Evie is dynamically typed with a lightweight set of built-in types:

| Type | Description | Example |
| :--- | :--- | :--- |
| `number` | 64-bit IEEE-754 floating point | `42`, `3.14`, `-0.5` |
| `bool` | Boolean | `true`, `false` |
| `string` | UTF-8 string | `"hello"`, `'world'` |
| `buffer` | Raw byte slice | `readFile("data.bin")` |
| `array` | Dynamic ordered collection | `[1, 2, "three"]` |
| `object` | Key-value associative map | `{ name: "Alice", age: 30 }` |
| `nil` | Represents absence of value | `nil` |
| `function` | First-class callable / closure | `fn(x) => x * 2` |
| `struct` | User-defined struct instance | `Person("Bob", 25)` |
| `task` | Asynchronous concurrent task | `go fn() { ... }` |
| `error` | Runtime or user exception | `error("failed")` |

### String Templates (Interpolation)
Backtick strings support inline expression interpolation using `{...}`:

```go
name := "Evie"
version := 0.1
greeting := `Welcome to {name} v{version}!`
io.println(greeting) // "Welcome to Evie v0.1!"
```

### Arrays
Arrays are zero-indexed, dynamic lists supporting negative indexing and element assignment:

```go
fruits := ["apple", "banana", "cherry"]

// Indexing
first := fruits[0]    // "apple"
last := fruits[-1]    // "cherry" (negative indexing from end)

// Subscript Mutation
var list := [10, 20, 30]
list[1] = 99          // list is now [10, 99, 30]
```

### Objects
Objects are flexible hash maps with string keys:

```go
user := {
    name: "Alice",
    age: 30,
    role: "admin"
}

// Field access and subscript access are interchangeable:
io.println(user.name)       // "Alice"
io.println(user["age"])     // 30

// Mutation:
var config := { debug: false }
config["debug"] = true
config["env"] = "production"
```

---

## 4. Operators

### Arithmetic & Modulo
```go
sum  := 10 + 5   // 15
diff := 10 - 5   // 5
prod := 10 * 5   // 50
quot := 10 / 5   // 2
rem  := 10 % 3   // 1
```

### Compound Assignment
```go
var x := 10
x += 5   // 15
x -= 2   // 13
x *= 2   // 26
x /= 2   // 13
x %= 4   // 1
```

### Comparisons
All standard comparison operators are supported:
```go
a == b    // equality
a != b    // inequality
a < b     // less than
a <= b    // less than or equal
a > b     // greater than
a >= b    // greater than or equal
```

### Logical Operators
Short-circuiting boolean operations:
```go
isReady && hasPermission  // logical AND
isCached || fetchRemote   // logical OR
!isCompleted              // logical NOT
```

### Type Checking (`is` Operator)
Evie provides a built-in `is` operator for safe runtime type assertions:

```go
42 is number           // true
"hello" is string      // true
true is bool           // true
[1, 2] is array        // true
{ a: 1 } is object     // true
t is task              // true
err is error           // true
buf is buffer          // true
p is Person            // true (struct instance check)
dog is Animal          // true (polymorphic inheritance check)
```

---

## 5. Functions & Closures

Functions in Evie are first-class values and can be passed around, returned, and stored.

### Function Declarations
```go
fn add(a, b) {
    return a + b
}
```

### Anonymous & Arrow Functions
Short single-expression functions can use the arrow `=>` syntax:

```go
// Block syntax
square := fn(x) {
    return x * x
}

// Arrow syntax (implicit return)
double := fn(x) => x * 2

// Ideal for higher-order functions:
evens := [1, 2, 3, 4].filter(fn(x) => x % 2 == 0)
```

### Variadic Functions (`...args`)
Functions can accept a variable number of trailing arguments:

```go
fn log(prefix, ...messages) {
    for msg in messages {
        io.println(prefix, msg)
    }
}

log("[INFO]", "Server started", "Listening on :8080")
```

### Closures
Functions capture bindings from enclosing scopes by reference:

```go
fn createCounter(initial) {
    var count := initial
    return fn() {
        count += 1
        return count
    }
}

counter := createCounter(10)
counter() // 11
counter() // 12
```

---

## 6. Control Flow

### If / Else
Like Go, parentheses around conditions are optional, but `{ ... }` blocks are strictly required:

```go
if score >= 90 {
    grade = "A"
} else if score >= 80 {
    grade = "B"
} else {
    grade = "C"
}
```

### While Loops
```go
var x := 5
while x > 0 {
    io.println(x)
    x -= 1
}
```

### For Loops (`for ... in`)
Evie features a unified `for ... in` loop supporting both single-element and indexed iteration across arrays, objects, strings, and buffers:

```go
// 1. Array iteration
for item in ["apple", "banana", "cherry"] {
    io.println(item)
}

// 2. Array iteration with index
for i, item in ["apple", "banana", "cherry"] {
    io.println(i, item) // 0 apple, 1 banana, 2 cherry
}

// 3. Object key iteration
for key in { a: 1, b: 2 } {
    io.println(key)
}

// 4. Object key & value iteration
for key, val in { a: 10, b: 20 } {
    io.println(key, "=", val)
}

// 5. String iteration (characters)
for ch in "Evie" {
    io.println(ch)
}

// 6. Break and Continue
for n in [1, 2, 3, 4, 5] {
    if n == 2 {
        continue
    }
    if n == 4 {
        break
    }
    io.println(n)
}
```

---

## 7. Structs & Object-Oriented Programming

Evie supports object-oriented programming with structs, encapsulated methods, explicit `this`, dynamic fields, first-class bound methods, and single inheritance.

### Struct Definition & Instantiation
Structs are declared with `struct Name(field1, field2, ...) { ... }`.
The body braces `{ ... }` are **mandatory** (even if empty). Constructor arguments passed during instantiation must match the number of declared fields.

```go
struct Person(name, age) {
    // Methods are declared directly inside the struct body
    fn greet(prefix) {
        return prefix + ", I am " + this.name
    }

    // Arrow syntax works for short methods
    fn isAdult() => this.age >= 18

    // Mutating struct fields via this
    fn haveBirthday() {
        this.age = this.age + 1
    }
}

// Instantiation:
p := Person("John", 22)
io.println(p.name) // "John"
io.println(p.age)  // 22

// Method calls:
io.println(p.greet("Hello")) // "Hello, I am John"
io.println(p.isAdult())      // true

p.haveBirthday()
io.println(p.age)            // 23
```

### Methods, `this`, & Bound Methods
- **The `this` Keyword**: Inside any struct method, `this` is an implicit receiver parameter referring to the instance.
- **Method Calls on `this`**: Methods can invoke other methods on the same instance using `this.otherMethod()`.
- **First-Class Bound Methods**: Accessing a method without calling it extracts a bound method holding the instance reference:

```go
greeter := Person("Alice", 30)
sayHi := greeter.greet // extracts bound method

io.println(sayHi("Hi")) // "Hi, I am Alice"
```

### Dynamic Fields
Struct instances are extensible. You can assign optional dynamic fields at runtime:

```go
p := Person("John", 22)
p.email = "john03@gmail.com" // dynamic field assignment

io.println(p.email)          // "john03@gmail.com"
```

### Classical Single Inheritance
Structs support single inheritance using `:` followed by the parent struct name (`struct Dog(...) : Animal { ... }`). Subclasses inherit parent methods, can override methods, and can add their own subclass-specific methods:

```go
struct Animal(name) {
    fn speak() {
        return this.name + " makes a generic animal sound"
    }

    fn describe() {
        return "Animal named " + this.name
    }
}

// Dog inherits from Animal
struct Dog(name, breed) : Animal {
    // Method override
    fn speak() {
        return this.name + " barks: Woof! Woof!"
    }

    // Subclass-specific method
    fn fetch(item) {
        return this.name + " fetched the " + item
    }
}

// Cat inherits from Animal
struct Cat(name) : Animal {
    // Method override
    fn speak() {
        return this.name + " purrs: Meow"
    }
}
```

Usage:
```go
a := Animal("Creature")
d := Dog("Rex", "German Shepherd")
c := Cat("Luna")

// Polymorphic method calls:
io.println(a.speak()) // "Creature makes a generic animal sound"
io.println(d.speak()) // "Rex barks: Woof! Woof!"
io.println(c.speak()) // "Luna purrs: Meow"

// Inherited methods from Animal:
io.println(d.describe()) // "Animal named Rex"
io.println(c.describe()) // "Animal named Luna"

// Subclass-specific method:
io.println(d.fetch("ball")) // "Rex fetched the ball"
```

### Polymorphic Type Checking (`is` Operator)
Subclasses satisfy both their own struct type and all ancestor struct types:

```go
d := Dog("Rex", "German Shepherd")
c := Cat("Luna")

d is Dog     // true
d is Animal  // true (inherited)
c is Animal  // true
!(c is Dog)  // true
```

---

## 8. Built-in Collection Methods

### Array Methods
```go
arr := [1, 2, 3]

arr.len()                 // 3
arr.push(4)               // [1, 2, 3, 4]
arr.pop()                 // returns 4, arr is now [1, 2, 3]
arr.slice(1, 3)           // [2, 3] (supports negative indices)
arr.contains(2)           // true
arr.reverse()             // [3, 2, 1]
arr.map(fn(x) => x * 10)  // [30, 20, 10]
arr.filter(fn(x) => x > 1)// [3, 2]
```

### Object Methods
```go
obj := { name: "John", age: 25 }

obj.keys()                // ["age", "name"] (sorted alphabetically)
obj.values()              // [25, "John"]
obj.len()                 // 2
obj.has("name")           // true
obj.remove("age")         // returns 25, removes key
obj.clear()               // removes all entries
```

### Buffer Methods
```go
buf := readFile("test.txt")
buf.len()                 // byte length
buf.string()              // string content
string(buf)               // string conversion constructor
```

---

## 9. Error Handling

Evie uses an explicit exception system with `throw`, `catch`, and an `error` constructor.

### Throwing Errors
```go
fn divide(a, b) {
    if b == 0 {
        throw error("division by zero")
    }
    return a / b
}
```

### Catching Errors
`catch(expr)` executes an expression and intercepts both runtime errors and user-thrown errors:

```go
result := catch(divide(10, 0))

if result is error {
    io.println("Failed:", result.message)
} else {
    io.println("Success:", result)
}
```

---

## 10. Concurrency: `go`, `await`, & `Task`

Evie provides built-in cooperative concurrency inspired by Go goroutines and JavaScript async/await.

### Spawning Tasks (`go`)
Use the `go` keyword followed by a function call to run it concurrently:

```go
task := go fn() {
    // runs concurrently
    return fetchRemoteData()
}
```

### Awaiting Tasks (`await`)
`await` pauses the current fiber until the task completes and unwraps the result:

```go
data := await task
```
- Multi-await memoization: Awaiting the same task multiple times immediately returns the memoized result without re-executing.
- Identity await: Awaiting a non-task value (e.g. `await 42`) is a safe no-op that returns the value.

### Multi-Task Concurrency (`await.all`, `await.any`, `await.race`)
```go
t1 := go fetchServiceA()
t2 := go fetchServiceB()

// Wait for all to succeed (returns array of results)
results := await.all(t1, t2)
// Or pass an array:
results = await.all([t1, t2])

// Wait for first successful task (ignores failures unless all fail)
firstSuccess := await.any(t1, t2)

// Wait for first task to settle (whether success or error)
winner := await.race(t1, t2)
```

### Task Methods
```go
t.done()       // returns a channel that closes when done
t.isDone()     // returns boolean
t.result()     // returns task result (or nil)
t.error()      // returns error (or nil)
t.await()      // method form of await
```

### Shared State & Synchronization (`synced`)
By default, Evie fibers run in true multi-threaded parallel execution across CPU cores without a Global Interpreter Lock. When multiple tasks access shared mutable variables, use `synced` blocks or `synced` functions to create mutually exclusive critical sections:

```go
var counter := 0

// 1. Synced block for fine-grained critical sections:
fn worker(n) {
    while n > 0 {
        synced {
            counter += 1 // Thread-safe mutation via runtime GIL
        }
        n -= 1
    }
}

// 2. Synced function:
fn deposit(amount) synced {
    counter += amount
}
```

- **Re-entrant**: Calling a `synced` block or function from within an already synchronized execution is a safe pass-through and will not deadlock.
- **Cooperative Await**: Awaiting a task inside `synced` code automatically releases the lock while blocked and re-acquires it once the task settles.

---

## 11. Go Host Embedding

Evie is designed to embed seamlessly in Go applications with bidirectional marshaling:

```go
package main

import (
    "fmt"
    "github.com/hxkhan/evie"
)

type OrderResult struct {
    Total   float64 `json:"total"`
    Message string  `json:"message"`
}

func main() {
    // 1. Create a VM instance
    env := evie.New()

    // 2. Expose Go functions directly to Evie
    env.Set("calculateTax", func(subtotal float64) float64 {
        return subtotal * 0.15
    })

    // 3. Run Evie scripts
    _, err := env.Run(`
    package main
    fn processOrder(price) {
        var tax := calculateTax(price)
        return {
            total: price + tax,
            message: "Order processed successfully"
        }
    }
    `)

    // 4. Call Evie functions from Go
    val, _ := env.Call("processOrder", 100.0)

    // 5. Unmarshal directly into a Go struct
    var res OrderResult
    val.Unmarshal(&res)

    fmt.Printf("Total: $%.2f, Status: %s\n", res.Total, res.Message)
    // Output: Total: $115.00, Status: Order processed successfully

    // 6. One-line quick evaluation
    ans, _ := evie.Eval("return [1, 2, 3].map(fn(x) => x * 10)")
    fmt.Println(ans) // [10, 20, 30]
}
```