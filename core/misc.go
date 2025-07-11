package core

import (
	"fmt"
	"strings"
	"unsafe"

	"github.com/hk-32/evie/op"
)

var runs [op.NUM_OPS]int

func WrapInstructions(before func(rt *CoRoutine), after func(rt *CoRoutine)) {
	for i, in := range instructions {
		instructions[i] = func(rt *CoRoutine) (v Value, err error) {
			runs[rt.vm.code[rt.ip]]++
			if before != nil {
				before(rt)
			}
			v, err = in(rt)
			if after != nil {
				after(rt)
			}
			return v, err
		}
	}
}

func PrintInstructionRuns() {
	for i, runs := range runs {
		if runs != 0 {
			fmt.Printf("%v : RAN(%v) \n", op.PublicName(byte(i)), runs)
		}
	}
}

func (m *Machine) DumpCode() {
	// number of digits for the biggest index
	width := digits(len(m.code))

	op.Walk(m.code, func(ip int) (size int) {
		switch b := m.code[ip]; b {
		case op.RETURN_IF:
			size := int(m.code[ip+1])
			fmt.Printf("%v : %v <%v>\n", padding_left(ip, width), op.PublicName(b), size)
			return 1 + 1

		case op.IF, op.ELIF, op.ELSE:
			size := int(uint16(m.code[ip+1]) | uint16(m.code[ip+2])<<8)
			fmt.Printf("%v : %v <%v>\n", padding_left(ip, width), op.PublicName(b), size)
			return 1 + 2

		case op.INT:
			num := *(*int64)(unsafe.Pointer(&m.code[ip+1]))
			fmt.Printf("%v : INT %v\n", padding_left(ip, width), num)
			return 1 + 8

		case op.FLOAT:
			num := *(*float64)(unsafe.Pointer(&m.code[ip+1]))
			fmt.Printf("%v : FLOAT %v\n", padding_left(ip, width), num)
			return 1 + 8

		case op.STR:
			size := int(*(*uint16)(unsafe.Pointer(&m.code[ip+1])))
			str := unsafe.String(&m.code[ip+3], size)
			fmt.Printf("%v : STR '%v'\n", padding_left(ip, width), str)
			return 1 + 2 + len(str)

		case op.LOAD_LOCAL, op.STORE_LOCAL, op.LOAD_CAPTURED, op.STORE_CAPTURED, op.LOAD_BUILTIN:
			name, _ := m.infoSource.GetSymbolName(ip)
			fmt.Printf("%v : %v %v\n", padding_left(ip, width), op.PublicName(b), name)
			return 1 + 1

		case op.FN_DECL:
			fn, _ := m.infoSource.GetFuncInfo(ip)
			fmt.Printf("%v : FN_DECL %v(%v) LOCALS(%v) ESC(%v) REFS(%v) <%v>\n", padding_left(ip, width), fn.Name, strings.Join(fn.Args, " "), fn.Capacity, fn.Capacity-len(fn.NonEscaping), len(fn.Refs), fn.End-ip)
			return 2
		case op.LAMBDA:
			fn, _ := m.infoSource.GetFuncInfo(ip)
			fmt.Printf("%v : LAMBDA (%v) LOCALS(%v) ESC(%v) REFS(%v) <%v>\n", padding_left(ip, width), strings.Join(fn.Args, " "), fn.Capacity, fn.Capacity-len(fn.NonEscaping), len(fn.Refs), fn.End-ip)
			return 1

		case op.CALL:
			nargs := byte(m.code[ip+1])
			fmt.Printf("%v : %v $%v\n", padding_left(ip, width), op.PublicName(b), nargs)
			return 1 + 1

		case op.AWAIT_ALL, op.AWAIT_ANY:
			nargs := byte(m.code[ip+1])
			fmt.Printf("%v : %v $%v\n", padding_left(ip, width), op.PublicName(b), nargs)
			return 1 + 1

		case op.GO:
			nargs := int(m.code[ip+1])
			fmt.Printf("%v : GO $%v\n", padding_left(ip, width), nargs)
			return 1 + 1

		default:
			fmt.Printf("%v : %v\n", padding_left(ip, width), op.PublicName(b))
			return 1
		}
	})
}

func padding_left(x int, space int) string {
	ast := fmt.Sprint(x)

	if len(ast) < space {
		amount := space - len(ast)
		return duplicate('0', amount) + ast
	}
	return ast
}

func digits(x int) int {
	if x == 0 {
		return 1
	}
	count := 0
	for x > 0 || x < 0 {
		x = x / 10
		count++
	}
	return count
}

func duplicate(x byte, num int) string {
	container := make([]byte, num)
	for n := 0; n < num; n++ {
		container[n] = x
	}
	return string(container)
}
