package main

import "github.com/hk-32/evie/rt"

/*
fn fib(x) {
	if (x < 2) {
		return x
	}
	return fib(x-1) + fib(x-2)
}

print(fib(35))
*/

func main() {
	defer rt.ExitMain()
	rt.GIL.Lock()

	var fib any
	fib = rt.NewFn("fib", func(x any) any {
		_line := -1
		defer rt.ExitFn("fib", &_line)

		_line = 1
		if rt.LESS(x, rt.Int(2)) {
			_line = 2
			return x
		}

		_line = 4
		return rt.ADD(
			rt.AsFn[func(any) any]("fib", fib)(rt.SUB(x, rt.Int(1))),
			rt.AsFn[func(any) any]("fib", fib)(rt.SUB(x, rt.Int(2))),
		)
	})

	rt.AsFn[func(any) any]("print", print)(rt.AsFn[func(any) any]("fib", fib)(rt.Int(35)))
}

/* Built-In Scope */

var print = rt.Println
var sleep = rt.Sleep
