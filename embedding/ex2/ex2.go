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

// The convenient way in: Go functions wrapped without reflection, a host
// package the script imports, Call with Go arguments, Unmarshal into a struct.
func main() {
	shop := vm.NewPackage("shop")
	shop.Native("greeting", vm.Func2(func(name string, count int) string {
		return fmt.Sprintf("Welcome %s! You have %d notifications.", name, count)
	}))
	shop.Set("taxRate", vm.BoxNumber(0.12))

	eng := evie.New(evie.Options{Packages: []*vm.Package{shop}})

	_, err := eng.Eval(`package main imports("io", "shop")

	fn calculateOrder(subtotal, count, customerName) {
		total := subtotal + subtotal * shop.taxRate
		msg := shop.greeting(customerName, count)
		io.println("[Evie]", msg)
		return { total: total, message: msg }
	}`)
	if err != nil {
		panic(err)
	}

	val, err := eng.Call("main.calculateOrder", 250.0, 3, "Alice")
	if err != nil {
		panic(err)
	}

	var result Result
	if err := val.Unmarshal(&result); err != nil {
		panic(err)
	}
	fmt.Printf("[Go] total = %.2f, message = %q\n", result.Total, result.Message)

	quick, _ := evie.Eval("return [1, 2, 3, 4, 5].filter(fn(x) => x > 2).map(fn(x) => x * 10)")
	fmt.Println("[Go] quick eval:", quick)
}
