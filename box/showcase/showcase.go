package main

import (
	"fmt"
	"unsafe"

	"github.com/hk-32/evie/box"
)

func main() {
	f1 := box.Float64(3.14)
	f2 := box.Float64(12)

	i1 := box.Int64(6)
	i2 := box.Int64(9)

	str := "hello world"
	fn := box.UserFn(unsafe.Pointer(new(byte)))

	b1 := box.Bool(true)
	b2 := box.Bool(false)

	// ------------------------

	fmt.Println(f1)
	fmt.Println(f2)

	fmt.Println(i1)
	fmt.Println(i2)

	fmt.Println(str)
	fmt.Println(fn)

	fmt.Println(b1)
	fmt.Println(b2)
}
