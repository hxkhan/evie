package evie_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hxkhan/evie"
	"github.com/hxkhan/evie/vm"
)

// Helper to run code and assert it completes successfully
func run(t *testing.T, code string) vm.Value {
	t.Helper()
	val, err := runCode(code)
	if err != nil {
		t.Fatalf("Execution failed: %v\nCode:\n%s", err, code)
	}
	return val
}

// Helper to evaluate Evie code and return the result value or error
func runCode(code string) (resVal vm.Value, retErr error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				retErr = e
			} else {
				retErr = fmt.Errorf("%v", r)
			}
		}
	}()

	evm := vm.New(evie.Defaults)
	trimmed := strings.TrimSpace(code)
	script := code
	if !strings.HasPrefix(trimmed, "package ") {
		if !strings.Contains(code, "return ") {
			script = fmt.Sprintf("package main imports(\"io\")\nfn main() {\n\treturn %s\n}", code)
		} else {
			script = fmt.Sprintf("package main imports(\"io\")\nfn main() {\n%s\n}", code)
		}
	}

	_, err := evm.EvalScript([]byte(script))
	if err != nil {
		return vm.Value{}, err
	}

	pkgMain := evm.GetPackage("main")
	if pkgMain == nil {
		return vm.Value{}, nil
	}

	symMain, exists := pkgMain.GetSymbol("main")
	if !exists {
		return vm.Value{}, nil
	}

	fn, ok := symMain.AsUserFn()
	if !ok {
		return vm.Value{}, fmt.Errorf("main is not a function")
	}

	fbr := evm.NewFiber()
	res, exc := fn.Call(fbr)
	evm.WaitForNoActivity()
	if exc != nil {
		return res, exc
	}
	return res, nil
}

func assertNumber(t *testing.T, val vm.Value, expected float64) {
	t.Helper()
	n, ok := val.AsFloat64()
	if !ok {
		t.Fatalf("expected number %v, got %s (%v)", expected, val.TypeOf(), val)
	}
	if n != expected {
		t.Fatalf("expected number %v, got %v", expected, n)
	}
}

func assertBool(t *testing.T, val vm.Value, expected bool) {
	t.Helper()
	b, ok := val.AsBool()
	if !ok {
		t.Fatalf("expected bool %v, got %s (%v)", expected, val.TypeOf(), val)
	}
	if b != expected {
		t.Fatalf("expected bool %v, got %v", expected, b)
	}
}

func assertString(t *testing.T, val vm.Value, expected string) {
	t.Helper()
	s, ok := val.AsString()
	if !ok {
		t.Fatalf("expected string %q, got %s (%v)", expected, val.TypeOf(), val)
	}
	if s != expected {
		t.Fatalf("expected string %q, got %q", expected, s)
	}
}

func TestArithmetic(t *testing.T) {
	assertNumber(t, run(t, "1 + 2"), 3)
	assertNumber(t, run(t, "10 - 4"), 6)
	assertNumber(t, run(t, "3 * 7"), 21)
	assertNumber(t, run(t, "20 / 4"), 5)
	assertNumber(t, run(t, "10 % 3"), 1)
	assertNumber(t, run(t, "2 + 3 * 4"), 14)
	assertNumber(t, run(t, "(2 + 3) * 4"), 20)
	assertNumber(t, run(t, "-5 + +10"), 5)
}

func TestComparisonsAndLogic(t *testing.T) {
	assertBool(t, run(t, "10 == 10"), true)
	assertBool(t, run(t, "10 != 20"), true)
	assertBool(t, run(t, "10 != 10"), false)
	assertBool(t, run(t, "10 < 20"), true)
	assertBool(t, run(t, "20 <= 20"), true)
	assertBool(t, run(t, "30 > 20"), true)
	assertBool(t, run(t, "30 >= 30"), true)
	assertBool(t, run(t, "true && true"), true)
	assertBool(t, run(t, "true && false"), false)
	assertBool(t, run(t, "true || false"), true)
	assertBool(t, run(t, "false || false"), false)
	assertBool(t, run(t, "!false"), true)
	assertBool(t, run(t, "!true"), false)
	assertBool(t, run(t, "!nil"), true)
	assertBool(t, run(t, "nil == nil"), true)
	assertBool(t, run(t, "true != false"), true)

	compound := `
	a := true
	b := false
	return !b && (a || b)
	`
	assertBool(t, run(t, compound), true)
}

func TestVariablesAndScoping(t *testing.T) {
	code1 := `
	x := 42
	return x
	`
	assertNumber(t, run(t, code1), 42)

	code2 := `
	var x := 10
	x = 20
	return x
	`
	assertNumber(t, run(t, code2), 20)

	code3 := `
	var x := 10
	x += 5
	x *= 2
	x -= 4
	x /= 2
	return x
	`
	assertNumber(t, run(t, code3), 13)
}

func TestControlFlow(t *testing.T) {
	codeIf := `
	x := 10
	if x > 5 {
		return 1
	} else {
		return 0
	}
	`
	assertNumber(t, run(t, codeIf), 1)

	codeWhile := `
	var i := 0
	var sum := 0
	while i < 10 {
		sum += i
		i += 1
	}
	return sum
	`
	assertNumber(t, run(t, codeWhile), 45)

	codeBreakContinue := `
	var i := 0
	var sum := 0
	while true {
		i += 1
		if i % 2 == 0 {
			continue
		}
		if i > 5 {
			break
		}
		sum += i
	}
	return sum
	`
	// odds: 1 + 3 + 5 = 9, then at 7 it breaks
	assertNumber(t, run(t, codeBreakContinue), 9)
}

func TestFunctionsAndClosures(t *testing.T) {
	codeNamedFn := `
	package main
	fn add(a, b) {
		return a + b
	}
	fn main() {
		return add(17, 25)
	}
	`
	assertNumber(t, run(t, codeNamedFn), 42)

	codeRecursion := `
	package main
	fn fib(n) {
		if n <= 1 {
			return n
		}
		return fib(n - 1) + fib(n - 2)
	}
	fn main() {
		return fib(8)
	}
	`
	assertNumber(t, run(t, codeRecursion), 21)

	codeClosure := `
	package main
	fn makeMultiplier(factor) {
		return fn(x) {
			return x * factor
		}
	}
	fn main() {
		triple := makeMultiplier(3)
		return triple(7)
	}
	`
	assertNumber(t, run(t, codeClosure), 21)
}

func TestArrays(t *testing.T) {
	codeLit := `
	arr := [10, 20, 30]
	return arr[0] + arr[1] + arr[2]
	`
	assertNumber(t, run(t, codeLit), 60)

	codeNegativeIndex := `
	arr := [10, 20, 30]
	return arr[-1]
	`
	assertNumber(t, run(t, codeNegativeIndex), 30)

	codeSubscriptAssign := `
	arr := [1, 2, 3]
	arr[1] = 50
	arr[-1] = 100
	return arr[0] + arr[1] + arr[2]
	`
	assertNumber(t, run(t, codeSubscriptAssign), 151)

	codeExistence := `
	arr := [1, 2, 3]
	return arr[0]? && arr[2]? && !arr[10]?
	`
	assertBool(t, run(t, codeExistence), true)

	codeNested := `
	m := [
		[1, 2],
		[3, 4],
	]
	return m[0][1] + m[1][0]
	`
	assertNumber(t, run(t, codeNested), 5)

	codeJoin := `
	arr := ["apple", "banana", "cherry"]
	return arr.join(", ")
	`
	assertString(t, run(t, codeJoin), "apple, banana, cherry")

	// Array is check
	assertBool(t, run(t, "[1, 2, 3] is array"), true)
	assertBool(t, run(t, `"hello" is array`), false)

	// Array len
	assertNumber(t, run(t, "[10, 20, 30, 40].len()"), 4)
	assertNumber(t, run(t, "[].len()"), 0)

	// Array push and append
	codePush := `
	arr := [1, 2]
	arr.push(3)
	arr.append(4)
	return arr.len() == 4 && arr[-1] == 4
	`
	assertBool(t, run(t, codePush), true)

	// Array pop
	codePop := `
	arr := [10, 20, 30]
	p := arr.pop()
	return p == 30 && arr.len() == 2
	`
	assertBool(t, run(t, codePop), true)

	// Array pop on empty returns nil
	codePopEmpty := `
	arr := []
	return arr.pop() == nil
	`
	assertBool(t, run(t, codePopEmpty), true)

	// Array slice
	codeSlice := `
	arr := [10, 20, 30, 40, 50]
	s1 := arr.slice(1, 4)
	s2 := arr.slice(1, -1)
	return s1.len() == 3 && s1[0] == 20 && s1[-1] == 40 && s2.len() == 3 && s2[-1] == 40
	`
	assertBool(t, run(t, codeSlice), true)

	// Array filter
	codeFilter := `
	arr := [1, 2, 3, 4, 5, 6]
	evens := arr.filter(fn(x) => x % 2 == 0)
	return evens.len() == 3 && evens[0] == 2 && evens[1] == 4 && evens[2] == 6
	`
	assertBool(t, run(t, codeFilter), true)

	// Array contains
	codeContains := `
	arr := ["cat", "dog", "bird"]
	return arr.contains("dog") && !arr.contains("fish")
	`
	assertBool(t, run(t, codeContains), true)

	// Array reverse
	codeReverse := `
	arr := [1, 2, 3]
	r := arr.reverse()
	return r[0] == 3 && r[1] == 2 && r[2] == 1
	`
	assertBool(t, run(t, codeReverse), true)

	// Array constructor
	codeConstructor := `
	a := array()
	return a is array && a.len() == 0
	`
	assertBool(t, run(t, codeConstructor), true)
}

func TestObjects(t *testing.T) {
	codeObj := `
	user := {name: "Alice", age: 30}
	return user.name
	`
	assertString(t, run(t, codeObj), "Alice")

	codeSubscript := `
	user := {name: "Alice", age: 30}
	user["city"] = "Paris"
	return user["city"]
	`
	assertString(t, run(t, codeSubscript), "Paris")

	codeObjExistence := `
	user := {name: "Alice"}
	return user["name"]? && !user["age"]?
	`
	assertBool(t, run(t, codeObjExistence), true)

	// Object methods: keys, values, len
	codeMethods := `
	user := {name: "Alice", age: 30}
	keys := user.keys()
	vals := user.values()
	return keys.len() == 2 && keys[0] == "age" && keys[1] == "name" && vals[0] == 30 && vals[1] == "Alice" && user.len() == 2
	`
	assertBool(t, run(t, codeMethods), true)

	// Object methods: has, contains
	codeHas := `
	user := {name: "Alice", age: 30}
	return user.has("name") && user.contains("age") && !user.has("city")
	`
	assertBool(t, run(t, codeHas), true)

	// Object methods: remove, delete
	codeRemove := `
	user := {name: "Alice", age: 30, city: "Paris"}
	val1 := user.remove("age")
	val2 := user.delete("city")
	return val1 == 30 && val2 == "Paris" && user.len() == 1 && !user.has("age") && !user.has("city")
	`
	assertBool(t, run(t, codeRemove), true)

	// Object methods: clear
	codeClear := `
	user := {name: "Alice", age: 30}
	user.clear()
	return user.len() == 0 && !user.has("name")
	`
	assertBool(t, run(t, codeClear), true)

	// object() constructor and is object
	codeConstructorAndType := `
	obj := object()
	obj["key"] = "value"
	isObj1 := obj is object
	isObj2 := {a: 1} is object
	isNotObj1 := [1, 2] is object
	isNotObj2 := "string" is object
	return isObj1 && isObj2 && !isNotObj1 && !isNotObj2 && obj["key"] == "value"
	`
	assertBool(t, run(t, codeConstructorAndType), true)

	// First-class method reference extraction
	codeBoundMethod := `
	user := {b: 2, a: 1}
	kFn := user.keys
	keys := kFn()
	return keys[0] == "a" && keys[1] == "b"
	`
	assertBool(t, run(t, codeBoundMethod), true)

	// Function stored inside object property
	codeObjFunction := `
	calc := {
		multiplier: 3,
		double: fn(x) { return x * 2 }
	}
	return calc.double(21)
	`
	assertNumber(t, run(t, codeObjFunction), 42)
}

func TestStructs(t *testing.T) {
	codeStruct := `
	package main
	struct Point(x, y) {}
	fn main() {
		p := Point(10, 20)
		return p.x + p.y
	}
	`
	assertNumber(t, run(t, codeStruct), 30)

	codeIsInstance := `
	package main
	struct Point(x, y) {}
	fn main() {
		p := Point(1, 2)
		return p is Point
	}
	`
	assertBool(t, run(t, codeIsInstance), true)

	// Struct method with this
	codeMethod := `
	package main
	struct Person(name, title) {
		fn greet(prefix) {
			return prefix + " " + this.name
		}
		fn getTitle() => this.title
	}
	fn main() {
		p := Person("Alice", "Engineer")
		return p.greet("Hello") + ", title: " + p.getTitle()
	}
	`
	assertString(t, run(t, codeMethod), "Hello Alice, title: Engineer")

	// Struct method mutating fields
	codeMutate := `
	package main
	struct Counter(count) {
		fn increment() {
			this.count = this.count + 1
		}
	}
	fn main() {
		c := Counter(10)
		c.increment()
		c.increment()
		return c.count
	}
	`
	assertNumber(t, run(t, codeMutate), 12)

	// Method calling another method on this
	codeMethodCallMethod := `
	package main
	struct Greeter(name) {
		fn base() => "Hello " + this.name
		fn excited() => this.base() + "!"
	}
	fn main() {
		g := Greeter("Bob")
		return g.excited()
	}
	`
	assertString(t, run(t, codeMethodCallMethod), "Hello Bob!")

	// Variadic struct method
	codeVariadicMethod := `
	package main
	struct Formatter(prefix) {
		fn format(...items) {
			var res := this.prefix
			var i := 0
			while i < items.len() {
				res = res + " " + items[i]
				i += 1
			}
			return res
		}
	}
	fn main() {
		f := Formatter("Log:")
		return f.format("test", "one", "two")
	}
	`
	assertString(t, run(t, codeVariadicMethod), "Log: test one two")

	// Bound method extracted and invoked
	codeBoundMethod := `
	package main
	struct Greeter(name) {
		fn greet(greeting) => greeting + ", " + this.name
	}
	fn main() {
		g := Greeter("Charlie")
		fnRef := g.greet
		return fnRef("Good morning")
	}
	`
	assertString(t, run(t, codeBoundMethod), "Good morning, Charlie")

	// Calling method with missing argument raises error
	codeInvalidArgCount := `
	package main
	struct Dog(name) {
		fn bark(sound) => this.name + " says " + sound
	}
	fn main() {
		d := Dog("Rex")
		return d.bark()
	}
	`
	_, err := runCode(codeInvalidArgCount)
	if err == nil {
		t.Fatal("expected error when calling method with missing arguments")
	}
}

func TestStructInheritance(t *testing.T) {
	codeInheritance := `
	package main
	struct Animal(name) {
		fn speak() => this.name + " makes a sound"
	}
	struct Dog(name, breed) : Animal {
		fn bark() => "Woof! I am " + this.name
	}
	struct Cat(name) : Animal {
		fn speak() => "Meow! I am " + this.name
	}
	fn main() {
		d := Dog("Buddy", "Golden")
		c := Cat("Whiskers")
		a := Animal("Creature")

		// Method inheritance
		dogSpeak := d.speak()
		dogBark := d.bark()

		// Method overriding
		catSpeak := c.speak()

		// Polymorphic is operator
		dIsDog := d is Dog
		dIsAnimal := d is Animal
		cIsCat := c is Cat
		cIsAnimal := c is Animal
		cIsDog := c is Dog
		aIsAnimal := a is Animal
		aIsDog := a is Dog

		return dogSpeak == "Buddy makes a sound" &&
			dogBark == "Woof! I am Buddy" &&
			catSpeak == "Meow! I am Whiskers" &&
			dIsDog && dIsAnimal &&
			cIsCat && cIsAnimal && !cIsDog &&
			aIsAnimal && !aIsDog
	}
	`
	assertBool(t, run(t, codeInheritance), true)

	// Multi-level inheritance
	codeMultiLevel := `
	package main
	struct Animal(name) {
		fn kind() => "animal: " + this.name
	}
	struct Mammal(name) : Animal {
		fn warmBlooded() => true
	}
	struct Dog(name) : Mammal {
		fn bark() => "woof"
	}
	fn main() {
		d := Dog("Rex")
		return (d is Dog) && (d is Mammal) && (d is Animal) &&
			d.warmBlooded() && (d.kind() == "animal: Rex")
	}
	`
	assertBool(t, run(t, codeMultiLevel), true)

	// Undefined parent struct error
	codeUndefinedParent := `
	package main
	struct Child() : NonExistentParent {}
	fn main() {}
	`
	_, err := runCode(codeUndefinedParent)
	if err == nil {
		t.Fatal("expected error for undefined parent struct")
	}

	// Cyclical inheritance error
	codeCycle := `
	package main
	struct Alpha() : Beta {}
	struct Beta() : Alpha {}
	fn main() {}
	`
	_, err = runCode(codeCycle)
	if err == nil {
		t.Fatal("expected error for cyclical inheritance")
	}
}

func TestStrings(t *testing.T) {
	assertString(t, run(t, `"hello " + "world"`), "hello world")
	assertString(t, run(t, `"abc"[1]`), "b")
	assertNumber(t, run(t, `"hello".len()`), 5)
	assertString(t, run(t, `"hello".toUpper()`), "HELLO")
	assertBool(t, run(t, `"hello".startsWith("he")`), true)
	assertBool(t, run(t, `"hello".endsWith("lo")`), true)
	assertBool(t, run(t, `"hello".contains("ell")`), true)
}

func TestBuffers(t *testing.T) {
	codeBuffer := `
	package main imports("fs")
	fn main() {
		buf := fs.readFile("go.mod")
		isBuf := buf is buffer
		isNotBuf := "hello" is buffer
		s1 := buf.string()
		s2 := string(buf)
		l := buf.len()
		return isBuf && !isNotBuf && s1 == s2 && l > 0 && s1.startsWith("module")
	}
	`
	assertBool(t, run(t, codeBuffer), true)
}

func TestErrorHandling(t *testing.T) {
	codeCatchRuntime := `
	b := catch(5 + "10")
	return b is error
	`
	assertBool(t, run(t, codeCatchRuntime), true)

	codeCatchThrow := `
	package main
	fn divide(a, b) {
		if b == 0 {
			throw "division by zero"
		}
		return a / b
	}
	fn main() {
		res := catch(divide(10, 0))
		if res is error {
			return res.message
		}
		return "success"
	}
	`
	assertString(t, run(t, codeCatchThrow), "division by zero")

	codeErrorConstructor := `
	e := error("my custom error")
	return e is error && e.message == "my custom error" && e.string() == "my custom error"
	`
	assertBool(t, run(t, codeErrorConstructor), true)

	// Verify uncaught throw propagates error up to Go caller
	_, err := runCode("throw \"uncaught exception\"")
	if err == nil {
		t.Fatal("expected error from uncaught throw, got nil")
	}
	if !strings.Contains(err.Error(), "uncaught exception") {
		t.Fatalf("expected error message to contain 'uncaught exception', got: %v", err)
	}
}

func TestConcurrency(t *testing.T) {
	codeGoAwait := `
	package main
	fn calculate() {
		return 42
	}
	fn main() {
		task := go calculate()
		return await task
	}
	`
	assertNumber(t, run(t, codeGoAwait), 42)

	// Multiple awaits on the same task (memoization)
	codeMultiAwait := `
	package main
	fn compute() {
		return 100
	}
	fn main() {
		t := go compute()
		r1 := await t
		r2 := await t
		r3 := t.await()
		return r1 == 100 && r2 == 100 && r3 == 100
	}
	`
	assertBool(t, run(t, codeMultiAwait), true)

	// Identity await on non-task values
	codeIdentityAwait := `
	package main
	fn main() {
		n := await 42
		s := await "hello"
		arr := await [1, 2, 3]
		return n == 42 && s == "hello" && arr.len() == 3
	}
	`
	assertBool(t, run(t, codeIdentityAwait), true)

	// Task methods and is task type check
	codeTaskMethods := `
	package main
	fn work() {
		return "done"
	}
	fn main() {
		t := go work()
		isT := t is task
		isNotT := "not a task" is task
		res := await t
		isFinished := t.done() && t.isDone()
		taskResult := t.result()
		return isT && !isNotT && res == "done" && isFinished && taskResult == "done"
	}
	`
	assertBool(t, run(t, codeTaskMethods), true)

	// Error propagation through task
	codeTaskError := `
	package main
	fn failing() {
		throw "task failed"
	}
	fn main() {
		t := go failing()
		err := catch(await t)
		return err is error && err.message == "task failed"
	}
	`
	assertBool(t, run(t, codeTaskError), true)

	// await.all with variadic and array form
	codeAwaitAll := `
	package main
	fn add(a, b) => a + b
	fn main() {
		t1 := go add(10, 20)
		t2 := go add(30, 40)
		res1 := await.all(t1, t2)
		tasks := [go add(1, 2), go add(3, 4)]
		res2 := await.all(tasks)
		return res1[0] == 30 && res1[1] == 70 && res2[0] == 3 && res2[1] == 7
	}
	`
	assertBool(t, run(t, codeAwaitAll), true)

	// await.any returns first successful task (even if another fails)
	codeAwaitAny := `
	package main
	fn slowSuccess() {
		return "winner"
	}
	fn failure() {
		throw "failed"
	}
	fn main() {
		t1 := go failure()
		t2 := go slowSuccess()
		res := await.any(t1, t2)
		tasks := [go failure(), go slowSuccess()]
		resArr := await.any(tasks)
		return res == "winner" && resArr == "winner"
	}
	`
	assertBool(t, run(t, codeAwaitAny), true)

	// await.race returns first settled task
	codeAwaitRace := `
	package main
	fn fast() => "first"
	fn slow() => "second"
	fn main() {
		t1 := go fast()
		t2 := go slow()
		res := await.race(t1, t2)
		tasks := [go fast(), go slow()]
		resArr := await.race(tasks)
		return (res == "first" || res == "second") && (resArr == "first" || resArr == "second")
	}
	`
	assertBool(t, run(t, codeAwaitRace), true)
}

func TestVariadicIO(t *testing.T) {
	code := `
	package main
	imports("io")
	fn main() {
		io.print()
		io.println()
		io.print("hello ")
		io.println("world")
		io.println("multiple", "arguments", 42, true)
		return true
	}
	`
	assertBool(t, run(t, code), true)
}

func TestVariadicUserFunctions(t *testing.T) {
	// Variadic function with no fixed args
	code1 := `
	package main
	fn sum(...nums) {
		var i := 0
		var total := 0
		while i < nums.len() {
			total = total + nums[i]
			i += 1
		}
		return total
	}
	fn main() {
		return sum(1, 2, 3, 4, 5)
	}
	`
	assertNumber(t, run(t, code1), 15)

	// Variadic function called with 0 args
	code2 := `
	package main
	fn countArgs(...args) {
		return args.len()
	}
	fn main() {
		return countArgs()
	}
	`
	assertNumber(t, run(t, code2), 0)

	// Variadic function with fixed args
	code3 := `
	package main
	fn format(prefix, sep, ...items) {
		var res := prefix
		var i := 0
		while i < items.len() {
			if i > 0 {
				res = res + sep
			}
			res = res + items[i]
			i += 1
		}
		return res
	}
	fn main() {
		return format("Values: ", ", ", "a", "b", "c")
	}
	`
	assertString(t, run(t, code3), "Values: a, b, c")

	// Variadic function with fixed args called with 0 variadic args
	code4 := `
	package main
	fn tag(name, ...items) {
		return items.len()
	}
	fn main() {
		return tag("title")
	}
	`
	assertNumber(t, run(t, code4), 0)

	// Anonymous / lambda variadic function
	code5 := `
	package main
	fn main() {
		f := fn(...args) => args.len()
		return f(10, 20, 30)
	}
	`
	assertNumber(t, run(t, code5), 3)

	// Variadic function in goroutine (go / await)
	code6 := `
	package main
	fn sumAsync(...nums) {
		var i := 0
		var total := 0
		while i < nums.len() {
			total = total + nums[i]
			i += 1
		}
		return total
	}
	fn main() {
		task := go sumAsync(10, 20, 30)
		return await task
	}
	`
	assertNumber(t, run(t, code6), 60)

	// Variadic function with insufficient arguments raises error
	code7 := `
	package main
	fn required(a, b, ...rest) {
		return a + b
	}
	fn main() {
		return required(1)
	}
	`
	_, err := runCode(code7)
	if err == nil {
		t.Fatal("expected error when calling variadic function with insufficient args")
	}

	// Variadic function passed to array filter
	code8 := `
	package main
	fn main() {
		nums := [1, 2, 3, 4, 5]
		evens := nums.filter(fn(...args) => args[0] > 2)
		return evens.len()
	}
	`
	assertNumber(t, run(t, code8), 3)
}

func TestExamples(t *testing.T) {
	exampleFiles := []string{
		"arrays.ev",
		"operators.ev",
		"errors.ev",
		"fib.ev",
		"functions.ev",
		"methods.ev",
		"struct.ev",
		"while.ev",
		"counter.ev",
		"captures.ev",
		"objects.ev",
		"variadic.ev",
		"inheritance.ev",
	}

	for _, file := range exampleFiles {
		t.Run(file, func(t *testing.T) {
			path := filepath.Join("examples", file)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("could not read %s: %v", path, err)
			}
			_, err = runCode(string(data))
			if err != nil {
				t.Fatalf("example %s failed: %v", file, err)
			}
		})
	}
}
