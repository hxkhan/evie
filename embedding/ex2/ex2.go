package main

import (
	"fmt"

	"github.com/hxkhan/evie"
)

type Result struct {
	Total   float64 `json:"total"`
	Message string  `json:"message"`
}

func main() {
	// 1. Create an Evie environment with standard library pre-configured
	env := evie.New()

	// 2. Expose a native Go function to Evie
	env.Set("formatGreeting", func(name string, count int) string {
		return fmt.Sprintf("Welcome %s! You have %d notifications.", name, count)
	})

	// 3. Expose host data
	env.Set("taxRate", 0.12)

	// 4. Run an Evie script defining functions and logic
	_, err := env.Run(`
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

	// 5. Call the Evie function directly from Go with automatic marshaling
	val, err := env.Call("calculateOrder", 250.0, 3, "Alice")
	if err != nil {
		panic(err)
	}

	// 6. Unmarshal directly into a Go struct
	var orderResult Result
	if err := val.Unmarshal(&orderResult); err != nil {
		panic(err)
	}

	fmt.Printf("[Go Host] Received: Total = $%.2f, Message = %q\n", orderResult.Total, orderResult.Message)

	// 7. One-liner script evaluation
	quickResult, _ := evie.Eval("return [1, 2, 3, 4, 5].filter(fn(x) => x > 2).map(fn(x) => x * 10)")
	fmt.Printf("[Go Host] Quick Eval: %v\n", quickResult)
}
