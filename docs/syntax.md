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

- Standard library packages (`io`, `fs`, `math`, `time`, `json`, see [section 12](#12-standard-library)) can be imported directly, as can packages the host registers and `.ev` files by relative path (`imports("./util")`).
- Every top-level function, class and binding of a package is visible to importers under the package name (`io.println`, `time.wait`). There is no private marker; keep internal helpers in a package that is not imported. An importer always sees a package's `var` bindings as they are now and can assign them (`util.count = 0`); assigning one of its constants is a compile error.
- Functions, classes and constants whose initializer is itself constant (`SECONDS := 60`, `MILLIS := SECONDS * 1000`) are hoisted, so they can be used before their declaration. Every other top-level binding is initialized in source order when the package loads; its initializer can call functions and build any value, and a binding read before its line has run is `nil`:

```go
package main imports("io")

config := { debug: false, retries: 3 }   // any expression, run at load time
limits := computeLimits(config.retries)

fn computeLimits(n) => [n, n * 2]

fn main() {
    io.println(config.debug, limits)
}
```

### Organizing a program

A package is one file or one folder; a program is the packages its entry point imports.

```
shop/
  main.ev        package main imports("./cart", "./util/money")
  checkout.ev    package main            (more of main)
  cart/
    cart.ev      package cart
    items.ev     package cart            (more of cart)
  util/
    money.ev     package money
```

- **A folder is one package.** Every `.ev` file in it declares the same name, and the files are compiled together, so any file can use what another declares.
- **A project's main package** is every top-level file that declares `package main`. `evie shop/` (or `eng.RunProject`) runs it; the other top-level files are packages imported by path. `evie file.ev` (or `eng.RunFile`) runs that one file.
- **Imports name a path or a package.** `"./cart"` and `"../lib/text"` are files or folders relative to the importing file, tried as `cart.ev` and then `cart/`. A bare name such as `"io"` is a package the host or the standard library provides. A local `math.ev` is imported as `"./math"`.
- **An import is bound by its alias or declared package name**: `imports("./util/money")` binds as `money`, and `imports("./util/money" as m)` binds as `m` (`as` is contextual inside `imports(...)` only).
- **Imports belong to the whole package**, so a package imports a module once, in any of its files.
- **Modules are identified by their canonical path, not only by short name.** Two different folders can both declare `package util` (or a local `./math.ev` can declare `package math` alongside stdlib `"math"`). If a single package imports both, give one an alias: `imports("math", "./math" as localMath)`. A folder whose files disagree on their package name, or importing a `main` file, is an error.
- **Errors inside an imported package** are reported in that package's file, with a note at the import.
- **An engine runs one program** and loads each module once; create a new engine for the next program (or parse once into an `*evie.Bundle` and run it across multiple engines).

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
| `string` | UTF-8 string | `"hello"`, `` `hi {name}` `` |
| `buffer` | Raw mutable byte slice | `buffer("hi")`, `fs.readFile("data.bin")` |
| `array` | Dynamic ordered collection | `[1, 2, "three"]` |
| `object` | Key-value map that keeps its keys in the order they were added | `{ name: "Alice", age: 30 }` |
| `nil` | Represents absence of value | `nil` |
| `function` | First-class callable / closure | `fn(x) => x * 2` |
| `class` | User-defined class instance | `Person("Bob", 25)` |
| `task` | Asynchronous concurrent task | `go fetchData()` |
| `error` | Runtime or user exception | `error("failed")` |

### Strings & Templates (Interpolation)
Strings are UTF-8 and operate on Unicode characters (runes) for indexing, slicing, iteration, and `.len()`:

```go
word := "café"
word.len()      // 4
word[3]         // "é"
word[-1]        // "é"
word[1:4]       // "afé"
```

Backtick strings support inline expression interpolation using `{...}`:

```go
name := "Evie"
version := 0.1
greeting := `Welcome to {name} v{version}!`
io.println(greeting) // "Welcome to Evie v0.1!"
```

### Escapes
Both kinds of string decode `\n` (newline), `\t` (tab), `\r`, `\0`, `\\` (a backslash) and `\"`. Templates also accept `` \` ``, `\{` and `\}` for a literal backtick or brace. Any other escape is a syntax error that points at it, so a Windows path is written `"C:\\Users"`:

```go
io.println("one\ttwo\nthree")
io.println(`braces: \{literal\}, value: {1 + 1}, 100%`)
```

### Arrays
Arrays are zero-indexed, dynamic lists supporting negative indexing, slicing, and element assignment:

```go
fruits := ["apple", "banana", "cherry", "date"]

// Indexing
first := fruits[0]    // "apple"
last := fruits[-1]    // "date" (negative indexing from end)

// Slicing (also works on strings and buffers)
mid := fruits[1:3]    // ["banana", "cherry"]
head := fruits[:2]    // ["apple", "banana"]
tail := fruits[-2:]   // ["cherry", "date"]

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

Compound assignment also works on object fields, class fields and subscripts. The target and key are evaluated once. Because only one fiber of an instance runs Evie code at a time, `this.count += 1` across concurrent tasks never loses updates:
```go
var stats := { hits: 0 }
stats.hits += 1
stats["hits"] *= 2

var scores := [10, 20, 30]
scores[0] += 5
scores[-1] -= 1

class Counter(count) {
    fn inc() {
        this.count += 1
    }
}
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
p is Person            // true (class instance check)
dog is Animal          // true (polymorphic inheritance check)
```

The type names double as conversion functions:
```go
number("2.5")   // 2.5 (throws if the string is not a number)
number(true)    // 1
bool(0)         // false (truthiness)
string(42)      // "42"
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

### Switch Statements
Evie supports multi-branch matching with `switch`:

```go
// 1. Value Switch with comma-separated matching
switch day {
case "monday", "tuesday", "wednesday", "thursday", "friday":
    io.println("weekday")
case "saturday", "sunday":
    io.println("weekend")
default:
    io.println("invalid day")
}

// 2. Tagless Switch (evaluates boolean conditions)
switch {
case score >= 90:
    grade = "A"
case score >= 80:
    grade = "B"
default:
    grade = "F"
}

// 3. Early Break
switch mode {
case 1:
    setup()
    break // terminates switch branch
    unreachable()
}
```
- Cases do not fall through by default.
- Multiple comma-separated values match if any value matches (`case 1, 2, 3:`).
- `break` inside a case exits the switch immediately without breaking enclosing loops.
- `case` blocks have their own lexical scopes, allowing local variable declarations.

---

## 7. Classes & Object-Oriented Programming

Evie supports object-oriented programming with fixed-shape classes, declared body fields (mutable or immutable), encapsulated methods, explicit `this`, first-class bound methods, and single inheritance.

### Class Definition & Instantiation
Classes are declared with `class Name(field1, field2, ...) { ... }`.
The body braces `{ ... }` are **mandatory** (even if empty). Constructor arguments passed during instantiation must match the number of constructor fields declared in parentheses.

```go
class Person(name, age) {
    // Body fields: immutable by default (:=) or mutable with var
    kind := "human"
    var email := ""

    // Methods are declared directly inside the class body
    fn greet(prefix) {
        return prefix + ", I am " + this.name
    }

    // Arrow syntax works for short methods
    fn isAdult() => this.age >= 18

    // Mutating instance fields via this
    fn haveBirthday() {
        this.age = this.age + 1
    }
}

// Instantiation:
p := Person("John", 22)
io.println(p.name) // "John"
io.println(p.age)  // 22
io.println(p.kind) // "human"

// Method calls:
io.println(p.greet("Hello")) // "Hello, I am John"
io.println(p.isAdult())      // true

p.haveBirthday()
io.println(p.age)            // 23
```

### Methods, `this`, & Bound Methods
- **The `this` Keyword**: Inside any class method (and in dynamic body field initializers), `this` is an implicit receiver parameter referring to the instance.
- **Method Calls on `this`**: Methods can invoke other methods on the same instance using `this.otherMethod()`.
- **First-Class Bound Methods**: Accessing a method without calling it extracts a bound method holding the instance reference:

```go
greeter := Person("Alice", 30)
sayHi := greeter.greet // extracts bound method

io.println(sayHi("Hi")) // "Hi, I am Alice"
```

### Fixed Shape & Body Fields
Classes have a **fixed field layout** known at compile time (constructor fields + body fields + inherited body fields). Assigning to an undeclared field is a runtime error; use an `object` (`{ ... }`) when you need arbitrary dynamic keys.

- **Immutable body fields (`field := expr`)**: initialized per instance and cannot be reassigned.
- **Mutable body fields (`var field := expr`)**: initialized per instance and can be mutated via `this.field = ...` or `instance.field = ...`.
- **Dynamic initializers**: non-constant expressions (such as `var items := []` or `full := this.first + " " + this.last`) are evaluated fresh for every new instance, so mutable defaults are never shared across instances.

```go
class User(first, last) {
    full := this.first + " " + this.last // immutable, computed per instance
    var email := ""                      // mutable with constant default
    var tags := []                       // fresh array per instance
}

u := User("Ada", "Lovelace")
u.email = "ada@example.com"
u.tags.push("math")
// u.full = "other"  // Error: cannot assign to constant field 'full' on class 'User'
// u.phone = "555"   // Error: class 'User' has no field 'phone'
```

### Classical Single Inheritance
Classes support single inheritance using `:` followed by the parent class name (`class Dog(...) : Animal { ... }`). Subclasses inherit parent methods, can override methods, and can add their own subclass-specific methods:

```go
class Animal(name) {
    fn speak() {
        return this.name + " makes a generic animal sound"
    }

    fn describe() {
        return "Animal named " + this.name
    }
}

// Dog inherits from Animal
class Dog(name, breed) : Animal {
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
class Cat(name) : Animal {
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

### Calling Parent Methods (`super`)
Inside a subclass method, use `super.method(...)` to call the parent class's implementation on the current `this` receiver, or `super.method` to extract a bound method:

```go
class LoudDog(name, breed) : Dog {
    fn speak() {
        return super.speak() + "!!!"
    }
}

io.println(LoudDog("Max", "Husky").speak()) // "Max barks: Woof! Woof!!!!"
```

### Polymorphic Type Checking (`is` Operator)
Subclasses satisfy both their own class type and all ancestor class types:

```go
d := Dog("Rex", "German Shepherd")
c := Cat("Luna")

d is Dog     // true
d is Animal  // true (inherited)
c is Animal  // true
!(c is Dog)  // true
```

### Classes and Concurrency
Class instances need no locking. Fibers of an instance never run at the same time, so a method like `fn inc() { this.count = this.count + 1 }` called from several tasks always counts every call.

---

## 8. Built-in Collection Methods

### Array Methods
```go
arr := [1, 2, 3]

arr.len()                 // 3
arr.push(4)               // mutates arr -> [1, 2, 3, 4] (alias: arr.append(4))
arr.pop()                 // returns 4, arr is now [1, 2, 3]
arr.slice(1, 3)           // [2, 3] (supports negative indices; end is optional)
arr.contains(2)           // true
["a", "b"].join("-")      // "a-b" (elements must be strings)
arr.reverse()             // returns new array [3, 2, 1] (does not mutate arr)
arr.map(fn(x) => x * 10)  // [10, 20, 30]
arr.filter(fn(x) => x > 1)// [2, 3]
```

### Object Methods
```go
obj := { name: "John", age: 25 }

obj.keys()                // ["name", "age"] (in the order the keys were added)
obj.values()              // ["John", 25] (in the same order)
obj.len()                 // 2
obj.has("name")           // true (alias: obj.contains("name"), or obj.name?)
obj.remove("age")         // returns 25, removes key (alias: obj.delete("age"))
obj.clear()               // removes all entries
```

### String Methods
Positions and lengths count characters (UTF-8 runes), not bytes:

```go
s := "  Hello, Evie!  "

s.len()                      // character (UTF-8 rune) count
s.trim()                     // "Hello, Evie!"
s.trimStart()                // "Hello, Evie!  "
s.trimEnd()                  // "  Hello, Evie!"
"v1.2".trimPrefix("v")       // "1.2"
"a.ev".trimSuffix(".ev")     // "a"
s.toUpper()                  // "  HELLO, EVIE!  "
s.toLower()                  // "  hello, evie!  "
s.contains("Evie")           // true
s.startsWith("  He")         // true
s.endsWith("!  ")            // true
"héllo".index("l")           // 2 (-1 if not found)
"hello".lastIndex("l")       // 3 (-1 if not found)
"banana".count("a")          // 3
"a-b-c".replace("-", "+")    // "a+b+c"
"a-b-c".replaceFirst("-", "+") // "a+b-c"
"ab".repeat(3)               // "ababab"
"héllo".reverse()            // "olléh"
"7".padStart(3, "0")         // "007"
"ab".padEnd(5, "xy")         // "abxyx"
"a,b,c".split(",")           // ["a", "b", "c"]
"  a  b c ".fields()         // ["a", "b", "c"]
"x\ny\n".lines()             // ["x", "y"]
"héy".chars()                // ["h", "é", "y"]
"café".slice(1, 4)           // "afé" (also s[1:4])
```

### Buffer Methods
```go
buf := buffer("hi")       // also buffer(), buffer(size), buffer([65, 66]), buffer(other)
file := fs.readFile("test.txt")

buf.len()                 // byte length (2)
buf[0]                    // 104 (byte value 0..255)
buf[0] = 72               // mutate byte in place ("Hi")
buf.push("!")             // append byte (0..255), string, or buffer (alias: buf.append)
buf.slice(0, 2)           // new buffer copy (also buf[0:2])
buf.string()              // "Hi!"
string(buf)               // "Hi!" (string conversion constructor)
```

---

## 9. Error Handling

Something that goes wrong is an `error` value. `throw` raises one, and `catch` turns one back into a value you can look at. Every error says what went wrong, what sort of failure it is, and where it happened.

### Throwing Errors
```go
fn divide(a, b) {
    if b == 0 {
        throw error("division by zero")
    }
    return a / b
}
```

`error(message, fields)` also takes an object of fields. `kind` names the sort of failure, `cause` is the error this one was raised because of, and any other name is data the catcher can read:

```go
throw error("cannot load the config", {kind: "config", path: path, cause: failure})
```

Throwing something that is not an error makes one: a string is the message, an object gives its fields as above (with `message` as the message), and any other value is kept as the field `value`. So `throw {kind: "http", code: 404}` arrives with `e.kind == "http"` and `e.code == 404`.

### Catching Errors
`catch(expr)` runs an expression and gives back its value, or the error it raised. `catch { ... }` does the same for a block of statements, and is `nil` when the block finishes without an error. A `return`, `break` or `continue` inside the block works as it would outside it.

```go
result := catch(divide(10, 0))
if result is error {
    io.println("Failed:", result.message, "at", result.file, result.line)
}

var config := {}
failure := catch {
    text := fs.readText("app.json")
    config = json.decode(text)
}
```

### What an Error Holds

| field | is |
| :-- | :-- |
| `message` | what went wrong |
| `kind` | the sort of failure, such as `"notFound"`; `"error"` unless one was given |
| `cause` | the error, or other value, this one was raised because of; `nil` when there is none |
| `file`, `line`, `column` | where it was thrown |
| `trace` | the functions that were running, from the innermost outwards, each with its position |
| any other | the fields it was given, as `e.path` above; `e.path?` tests for one |

Errors are values: they can be stored, passed around and thrown again. Throwing a caught error keeps its original position and trace.

### Kinds
The kind is how code tells failures apart, without depending on the wording of a message. Evie and its standard library use these:

| kind | for |
| :-- | :-- |
| `"type"` | a value of the wrong type, a call with the wrong number of arguments, a constant assigned |
| `"value"` | the right type but an unusable value, as in `number("two")` |
| `"range"` | an index outside an array, string or buffer |
| `"member"` | a field, key, method or package member that does not exist |
| `"notFound"`, `"exists"`, `"permission"`, `"io"` | the `fs` package: a missing file, one that already exists, access not allowed, any other failure; the error's `path` names the file |
| `"timeout"` | `await.timeout` that ran out of time |
| `"cancelled"` | a task that was cancelled |
| `"syntax"` | text that does not parse, for `json.decode` and `time.parse` |
| `"stackOverflow"` | too many calls running at once, almost always recursion that never stops; the limit is 10,000 unless the host sets another |
| `"error"` | everything else, and errors made without a kind |

Your own kinds work the same way:

```go
fn load(path) {
    text := catch(fs.readText(path))
    if text is error {
        throw error("cannot load the config", {kind: "config", path: path, cause: text})
    }
    return json.decode(text)
}

e := catch(load("app.json"))
if e is error && e.cause.kind == "notFound" {
    io.println("no config at", e.path, "- using the defaults")
}
```

### Uncaught Errors
Uncaught errors reach the host with the same information. The `evie` command prints them with their kind, unless it is `"error"`, the offending line, the span underlined and labelled, related places, a hint when one applies, and the trace:

```
error[type]: cannot apply '+' to a number and a string
 --> example.ev:6:9
  |
6 |         sum += v
  |         ^^^^^^^^ number + string
  |
  = help: convert the other side with string(...) to concatenate, or with number(...) to add
  = trace: average at example.ev:6:9
           report at example.ev:11:22
           main at example.ev:14:5
```

A Go host gets the same rendering from any error that implements `Diagnostic() *diag.Report`: `report.Render(diag.Colors)` for a terminal, `diag.Palette{}` for plain text, and `err.Error()` for the one-line form with the location in parentheses. The host also reads an error's kind, fields and cause, and gives errors its own (see Go Host Embedding).

---

## 10. Concurrency: `go`, `await`, & `Task`

Evie provides built-in cooperative concurrency inspired by Go goroutines and JavaScript async/await.

### Spawning Tasks (`go`)
Use the `go` keyword followed by a function call to run it concurrently:

```go
task := go fetchRemoteData()

// Or invoke an inline anonymous function:
inline := go fn() {
    return fetchRemoteData()
}()
```

### Awaiting Tasks (`await`)
`await` pauses the current fiber until the task completes and unwraps the result:

```go
data := await task
```
- Multi-await memoization: Awaiting the same task multiple times immediately returns the memoized result without re-executing.
- Identity await: Awaiting a non-task value (e.g. `await 42`) is a safe no-op that returns the value.

### Multi-Task Concurrency (`await.all`, `await.any`, `await.race`, `await.timeout`)
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

// Wait for a task with a deadline in milliseconds (cancels and throws on timeout)
bounded := await.timeout(t1, 500)
```

### Task Methods
```go
t.done()       // returns boolean: whether the task has settled
t.isDone()     // alias for t.done()
t.result()     // returns task result (or nil)
t.error()      // returns error (or nil)
t.await()      // method form of await
t.cancel()     // requests cooperative cancellation of the task
```

### Shared State
Only one fiber of an instance executes Evie code at a time. The rule for when it hands over is short: **a fiber yields only when it waits**. That happens in exactly three places:

- it **awaits** a task, in any form (`await t`, `await.all`, `await.any`, `await.race`, `await.timeout`, `t.await()`);
- it **finishes**, by returning or throwing;
- it calls a function that **waits on the outside world**: `time.wait`, `io.prompt`, `io.readln` and the `fs` functions that touch the disk. The Standard Library tables mark these **yields**; no other standard function yields. A Go host can add its own (see `Blocking` under Go Host Embedding).

Nothing else hands over: arithmetic, assignments, method calls, `go` itself, `io.println`, `json`, `math`. A call to one of your own functions yields only if it reaches one of the three places, as a helper that calls `time.wait` does. Between those points a fiber runs uninterrupted, so shared variables, arrays, objects and classes never need locks:

```go
var counter := 0

fn worker(n) {
    while n > 0 {
        counter += 1 // never lost: no other fiber runs until this one awaits
        n -= 1
    }
}

fn main() {
    await.all(go worker(500), go worker(500))
    return counter // always 1000
}
```

- **No data races**: a script cannot corrupt memory or crash the host by sharing values between tasks.
- **Cooperative handover**: a task spawned with `go` starts running when the current fiber yields: it awaits, finishes, or calls a function that waits.
- **Parallelism across instances**: to use several cores, a host runs several `vm.Instance`s. Each has its own heap; nothing is shared between them.

---

## 11. Go Host Embedding

Evie is designed to embed seamlessly in Go applications with bidirectional marshaling:

```go
package main

import (
    "fmt"
    "github.com/hxkhan/evie"
    "github.com/hxkhan/evie/vm"
)

type OrderResult struct {
    Total   float64 `json:"total"`
    Message string  `json:"message"`
}

func main() {
    // 1. An engine; the prelude holds symbols every script sees without an import
    eng := evie.New()
    defer eng.Close()
    eng.SetStatic("calculateTax", vm.BoxGoFunc(vm.Func1(func(subtotal float64) float64 {
        return subtotal * 0.15
    })))

    // 2. Load a script
    _, err := eng.Eval(`package main
    fn processOrder(price) {
        tax := calculateTax(price)
        return { total: price + tax, message: "Order processed successfully" }
    }`)
    if err != nil {
        panic(err)
    }

    // 3. Call it with Go arguments and unmarshal the result into a struct
    val, err := eng.Call("main.processOrder", 100.0)
    if err != nil {
        panic(err)
    }
    var res OrderResult
    if err := val.Unmarshal(&res); err != nil {
        panic(err)
    }
    fmt.Printf("Total: $%.2f, Status: %s\n", res.Total, res.Message)
    // Output: Total: $115.00, Status: Order processed successfully

    // 4. One-line quick evaluation
    ans, err := evie.Eval("return [1, 2, 3].map(fn(x) => x * 10)")
    if err != nil {
        panic(err)
    }
    fmt.Println(ans) // [10, 20, 30]
}
```

The pieces, from most to least convenient:

| need | use |
| :-- | :-- |
| expose a Go function | `vm.Func0`..`Func3` and the `E` variants for `(R, error)`; `vm.Reflect` for any other signature |
| full control of arguments and errors | `pkg.Func(name, arity, func(c vm.Call) (vm.Value, error))`; mark it `Blocking` if it sleeps or waits on I/O, and watch `c.Context()` so it wakes when the script is stopped |
| a group of functions scripts import by name | `vm.NewPackage("name")` passed in `evie.Options.Packages`; `pkg.Document(name, "f(x)", "what it does")` describes a member for editors, which `pkg.Describe(name)` reads back |
| check a program without running it | `eng.Check(b)` or `evie.Check(b)` compile a bundle and its imports and report their compile errors, up to ten per package; nothing runs |
| learn what every name refers to, for an editor | `evie.Options.OnName` receives a `vm.NameEvent` for each name the compiler declares or resolves: its kind and where it is declared, across files |
| sandbox stdlib, filesystem imports, or I/O per engine | `evie.Options.FS` (`embed.FS` / `io/fs.FS`; confines imports, and `fs` reads only from it and cannot write), `Options.Stdlib` (allowlist), `Options.NoStdlib`, `Options.Stdin`, `Options.Stdout` |
| parse a multi-file program once and run in many engines | `evie.ParseFile(fsys, path)`, `ParseProject(fsys, dir)` (a nil `fsys` is the OS) or `ParseSource(name, src)` → `*evie.Bundle`, then `eng.RunBundle(ctx, b)` or `eng.LoadBundle(ctx, b)` |
| symbols visible without an import | `eng.SetStatic` or `evie.Options.Prelude` |
| lend a Go value to scripts as it is | `vm.NewHostType[T]("Name")` with `Method`, `Constructor`, `Wrap` |
| a native whose work takes a while | return `c.Async(func(ctx context.Context) (vm.Value, error) { ... })` (lifecycle-bound task) |
| call a script function | `eng.Call("pkg.fn", goArgs...)`, or `value.Call(evieArgs...)` on a value from `eng.Lookup("pkg.fn")` |
| run a file or a project directory | `eng.RunFile`, `eng.RunProject`, or `evie.RunFile` for a one-off |
| read a result | `AsFloat64`, `AsString`, `AsArray`, `AsObject` and the other `As` methods, `Field(name)` / `ClassName()` on objects and class instances, `ToGo`, `Unmarshal(&target)`, `json.Marshal(val)` |
| compare values the way the language does | `a.Equals(b)`, `v.IsTruthy()` |
| show an error | `evie.Format(err, colors)` renders it the way the `evie` command does |
| raise an error scripts can tell apart | return `vm.Errorf(vm.KindNotFound, "no user %d", id).With("id", vm.BoxNumber(id))` from a native; a plain Go error works too and takes its kind from what it wraps, as `"notFound"` from `fs.ErrNotExist` and `"timeout"` from `context.DeadlineExceeded` |
| read an error a script raised | `errors.As(err, &e)` with `e *vm.Error`, then `e.Kind()`, `e.Field(name)`, `e.FieldNames()` and `e.CauseValue()`; `errors.Is` sees through the causes a script gave |
| stop a runaway script | `eng.Interrupt()`, or a context: `eng.RunFileContext`, `EvalContext`, `CallContext`, `RunBundle(ctx, b)` |
| limit how deep scripts recurse | `evie.Options.MaxCallDepth` (default 10,000); a call past it raises a catchable `"stackOverflow"` error, so runaway recursion cannot crash the host |
| finish with an engine | `eng.Wait()` lets background tasks finish; `eng.Close()` stops them, waits, and makes later calls return `evie.ErrClosed` |

A stopped script fails at its next loop iteration or call, or as soon as it wakes from an `await`, `time.sleep` or a blocking native that watches `c.Context()`. The error says where it stopped, its cause is `vm.ErrInterrupted` or the context's cause, and `catch` lets it pass. A context stops only the run it was given and the tasks that run started; `Interrupt` stops everything the instance is running, and the instance stays usable afterwards.

```go
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
defer cancel()
if _, err := eng.RunFileContext(ctx, "main.ev"); errors.Is(err, context.DeadlineExceeded) {
    fmt.Print(evie.Format(err, false)) // error[timeout]: context deadline exceeded, at the loop it was in
}
```

---

## 12. Standard Library

Import a package by name: `package main imports("math", "json")`. Functions marked **yields** wait on the outside world and let other tasks run while they wait (see Shared State under Concurrency); no other standard function yields. Arguments of the wrong type raise an error that names the function and the argument.

### `io`
Reads and writes use the engine's `Options.Stdin` and `Options.Stdout` (defaulting to `os.Stdin` and `os.Stdout`).

| function | does |
| :-- | :-- |
| `print(...values)`, `println(...values)` | write the values separated by spaces |
| `prompt(message)` | print a message and read a line (`nil` at EOF); **yields** |
| `readln()` | read a line (`nil` at EOF); **yields** |

### `math`
| function | does |
| :-- | :-- |
| `pi`, `e`, `inf`, `nan`, `maxSafeInt` | constants |
| `abs`, `sign`, `floor`, `ceil`, `round`, `trunc` | rounding and sign |
| `sqrt`, `cbrt`, `pow(x, y)`, `exp`, `log`, `log2`, `log10`, `hypot(x, y)` | powers and logarithms |
| `sin`, `cos`, `tan`, `asin`, `acos`, `atan`, `atan2(y, x)`, `sinh`, `cosh`, `tanh` | trigonometry, in radians |
| `min(...)`, `max(...)` | smallest or largest of the arguments, or of one array |
| `clamp(x, lo, hi)` | `x` limited to the range |
| `isNaN(x)`, `isInf(x)`, `isFinite(x)` | checks |
| `random()`, `randomInt(lo, hi)` | a number in [0, 1), an integer in [lo, hi] |

### `fs`
| function | does |
| :-- | :-- |
| `readFile(path)`, `readText(path)` | a file as a buffer or as a string; **yields** |
| `writeFile(path, content)`, `appendFile(path, content)` | write or append a string or buffer; **yields** |
| `exists(path)`, `isDir(path)` | tests; **yields** |
| `list(dir)` | sorted names in a directory; **yields** |
| `mkdir(dir)` | create a directory and its parents; **yields** |
| `remove(path)`, `rename(from, to)` | remove a file or directory tree, move it; **yields** |
| `join(...parts)`, `base(path)`, `dir(path)`, `ext(path)` | path helpers; these do not touch the disk and do not yield |

### `time`
Times are milliseconds since the Unix epoch, so they compare and subtract like any number.

| function | does |
| :-- | :-- |
| `now()`, `since(t)` | the current time, milliseconds elapsed since `t` |
| `clock()` | a monotonic clock in milliseconds, for measuring |
| `wait(ms)` (also `sleep`, `timer`) | pause the current task; **yields** |
| `format(t)`, `format(t, layout)` | UTC RFC 3339 text, or a [Go layout](https://pkg.go.dev/time#pkg-constants) such as `"2006-01-02"` |
| `parse(text)`, `parse(text, layout)` | parse an RFC 3339 time or a custom Go layout |
| `second`, `minute`, `hour`, `day` | durations in milliseconds |

### `json`
| function | does |
| :-- | :-- |
| `json.encode(value)`, `json.decode(text)` | convert between values and JSON text (rejects NaN/Infinity, functions, tasks, errors, and cycles) |
