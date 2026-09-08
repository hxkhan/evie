package main

import (
	"fmt"

	"github.com/hxkhan/evie"
	"github.com/hxkhan/evie/vm"
)

type Result struct {
	Total   float64 `json:"total"`
	Message string  `json:"message"`
}

func main() {
	// 1. Create an Evie environment with standard library pre-configured
	env := evie.New()

	// 2. Expose a native Go function to Evie
	greetingFn, err := vm.BoxAny(func(name string, count int) string {
		return fmt.Sprintf("Welcome %s! You have %d notifications.", name, count)
	})
	if err != nil {
		panic(err)
	}
	env.SetStatic("formatGreeting", greetingFn)

	// 3. Expose host data
	env.SetStatic("taxRate", vm.BoxNumber(0.12))

	// 4. Run an Evie script defining functions and logic
	_, err = env.EvalString(`
	package main imports("io")

	fn calculateOrder(subtotal, count, customerName) {
		var total := subtotal + (subtotal * taxRate)
		var msg := formatGreeting(customerName, count)

		io.println("[Evie]", msg)
		io.println("[Evie] Order total with tax:", total)

		return {
			total: total,
			message: msg
		}
	}
	`)
	if err != nil {
		panic(err)
	}

	// 5. Look up the function reference once
	calcOrder, ok := env.Lookup("main.calculateOrder")
	if !ok || !calcOrder.IsCallable() {
		panic("function calculateOrder not found or not callable")
	}

	// 6. Call the Evie function directly using its reference and a Fiber
	fbr := env.NewFiber()
	val, exc := calcOrder.Call(fbr, vm.BoxNumber(250.0), vm.BoxNumber(3), vm.BoxString("Alice"))
	if exc != nil {
		panic(exc)
	}

	// 7. Unmarshal directly into a Go struct
	var orderResult Result
	if err := val.Unmarshal(&orderResult); err != nil {
		panic(err)
	}

	fmt.Printf("[Go Host] Received: Total = $%.2f, Message = %q\n", orderResult.Total, orderResult.Message)

	// 8. One-liner script evaluation
	quickResult, _ := evie.Eval("return [1, 2, 3, 4, 5].filter(fn(x) => x > 2).map(fn(x) => x * 10)")
	fmt.Printf("[Go Host] Quick Eval: %v\n", quickResult)
}
