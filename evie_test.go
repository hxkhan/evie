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
