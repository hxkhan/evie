package ast

import (
	"fmt"
	"strings"
	"unsafe"

	"github.com/hk-32/evie/core"
	"github.com/hk-32/evie/op"
)

func NewCompiler(exports map[string]core.Value) *CompilerState {
	cs := &CompilerState{
		globals:              make(map[string]int),
		fns:                  make(map[int]*core.FuncInfo),
		symbols:              make(map[int]string),
		rcRoot:               &reachability{[]map[string]int{make(map[string]int, len(exports))}, 0, 0, nil},
		uninitializedGlobals: make(map[string]struct{}),
		optimise:             true,
	}
	cs.rc = cs.rcRoot

	cs.builtins = make([]core.Value, len(exports))
	for name, value := range exports {
		cs.builtins[cs.declare(name)] = value
	}
	// extend from builtin to global scope
	cs.scopeExtend()

	cs.vm = core.NewMachine(cs.builtins, cs)
	return cs
}

func (cs *CompilerState) Compile(node Node) (core.Value, error) {
	node.compile(cs)
	return cs.vm.Run(cs.output, len(cs.globals))
}

func (cs *CompilerState) GetSymbolName(ip int) (symbol string, exists bool) {
	symbol, exists = cs.symbols[ip]
	return
}

func (cs *CompilerState) GetFuncInfo(ip int) (info *core.FuncInfo, exists bool) {
	info, exists = cs.fns[ip]
	return
}

func (cs *CompilerState) GetGlobalAddress(name string) (addr int, exists bool) {
	addr, exists = cs.globals[name]
	return
}

func (cs *CompilerState) GetVM() *core.Machine {
	return cs.vm
}

func (cs *CompilerState) BuiltIns() []core.Value {
	return cs.builtins
}

func (cs *CompilerState) ReferenceTable() map[int]string {
	return cs.symbols
}

func (cs *CompilerState) FuncTable() map[int]*core.FuncInfo {
	return cs.fns
}

type Package struct {
	Name    string
	Imports []string
	Code    []Node
}

/*
Hoisting rules etc.
All symbols are first symbolically pre-declared without initialization.
This is so when we later initialize them; they can reference each other.
Then function initializations are physically moved to the top of the code.
And finally the rest of the code follows right after.

So this is not possible because the order is maintained:

	x := y + 2
	y := 10

But this is; becuase the declaration ends up being shifted to the top:

	x := 10
	echo x
*/
func (p Package) compile(cs *CompilerState) int {
	// 1. declare all symbols
	for _, node := range p.Code {
		if fnDec, isFnDecl := node.(Fn); isFnDecl {
			cs.globals[fnDec.Name] = cs.declare(fnDec.Name)
		}

		if iGet, isIdentDec := node.(IdentDec); isIdentDec {
			cs.uninitializedGlobals[iGet.Name] = struct{}{}
			cs.globals[iGet.Name] = cs.declare(iGet.Name)
		}
	}

	// 2. physically move function initialization to the top
	for _, node := range p.Code {
		if fnDec, isFnDecl := node.(Fn); isFnDecl {
			fnDec.compileInGlobal(cs)
		}
	}

	// compile the rest of the code
	for _, node := range p.Code {
		if _, isFnDecl := node.(Fn); isFnDecl {
			continue
		}

		// compile global variable declarations in a special way
		if iDec, isIdentDec := node.(IdentDec); isIdentDec {
			iDec.compileInGlobal(cs)
			delete(cs.uninitializedGlobals, iDec.Name)
			continue
		}
		node.compile(cs)
	}

	return 0
}

type reachability struct {
	lookup   []map[string]int
	index    int
	cap      int
	previous *reachability
}

func (rc *reachability) String() string {
	s := strings.Builder{}
	s.WriteByte('{')

	for _, lookup := range rc.lookup {
		n := 0
		for name, index := range lookup {
			n++
			s.WriteString(fmt.Sprintf("%v: %v", name, index))
			if n != len(lookup) {
				s.WriteString(", ")
			}
		}
	}
	s.WriteByte('}')

	if rc.previous != nil {
		s.WriteString(" -> ")
		s.WriteString(rc.previous.String())
	}
	return s.String()
}

type CompilerState struct {
	vm      *core.Machine
	output  []byte
	globals map[string]int

	builtins      []core.Value
	fns           map[int]*core.FuncInfo
	symbols       map[int]string
	openFunctions []struct {
		ip            int
		escapedLocals map[int]struct{}
	} // open functions and their escape'e locals

	rc                   *reachability       // current scope
	rcRoot               *reachability       // built-in scope
	uninitializedGlobals map[string]struct{} // IDEA: make it initializedGlobals of type map[string]bool
	optimise             bool
}

func (cs *CompilerState) emit(bytes ...byte) (pos int) {
	pos = len(cs.output)
	cs.output = append(cs.output, bytes...)
	return pos
}

func (cs *CompilerState) set(ip int, b byte) {
	cs.output[ip] = b
}

func (cs *CompilerState) emitString(str string) (pos int) {
	pos = len(cs.output)
	size := uint16(len(str))
	cs.output = append(cs.output, op.STR)
	cs.output = append(cs.output, byte(size))
	cs.output = append(cs.output, byte(size>>8))
	cs.output = append(cs.output, str...)
	return pos
}

func (cs *CompilerState) emitInt64(n int64) (pos int) {
	pos = len(cs.output)
	cs.output = append(cs.output, op.INT)
	cs.output = append(cs.output, (*[8]byte)(unsafe.Pointer(&n))[:]...)
	return pos
}

func (cs *CompilerState) emitFloat64(n float64) (pos int) {
	pos = len(cs.output)
	cs.output = append(cs.output, op.FLOAT)
	cs.output = append(cs.output, (*[8]byte)(unsafe.Pointer(&n))[:]...)
	return pos
}

func (cs *CompilerState) len() int {
	return len(cs.output)
}

func (cs *CompilerState) scopeExtend() {
	cs.rc = &reachability{lookup: []map[string]int{{}}, previous: cs.rc}
	//fmt.Println("AFTER scopeExtend():", s.rc)
}

func (cs *CompilerState) scopeDeExtend() {
	cs.rc = cs.rc.previous
	//fmt.Println("AFTER scopeDeExtend():", s.rc)
}

func (cs *CompilerState) scopeCapacity() int {
	return max(cs.rc.index, cs.rc.cap)
}

func (cs *CompilerState) scopeOpenBlock() {
	cs.rc.lookup = append(cs.rc.lookup, map[string]int{})
}

func (cs *CompilerState) scopeCloseBlock() {
	cs.rc.lookup = cs.rc.lookup[:len(cs.rc.lookup)-1]
}

func (cs *CompilerState) scopeReuseBlock() {
	top := cs.rc.lookup[len(cs.rc.lookup)-1]
	// save current cap, might be bigger than the reused cap later; in that case, we want the biggest
	if cs.rc.cap < cs.rc.index {
		cs.rc.cap = cs.rc.index
	}
	cs.rc.index -= len(top)
	for k := range top {
		delete(top, k)
	}
}

func (cs *CompilerState) declare(name string) (index int) {
	scope := cs.rc.lookup[len(cs.rc.lookup)-1]
	if _, exists := scope[name]; exists {
		panic(fmt.Sprintf("declare(\"%v\") -> double declaration of symbol!", name))
	}
	scope[name] = cs.rc.index
	cs.rc.index++
	return cs.rc.index - 1
}

// like reach but it has to be already declared alr locally
func (cs *CompilerState) get(name string) (index int) {
	scope := cs.rc.lookup[len(cs.rc.lookup)-1]
	if i, exists := scope[name]; exists {
		return i
	}
	panic("get() -> why is it not declared already?")
}

func (cs *CompilerState) reach(name string) core.Reference {
	this := cs.rc
	for scroll := 0; this != nil; scroll++ {
		for i := len(this.lookup) - 1; i >= 0; i-- {
			if index, exists := this.lookup[i][name]; exists {
				// if built-in scope then return scroll -1 to signal that
				if this.previous == nil {
					return core.Reference{Index: index, Scroll: -1}
				}
				// if accessing global from global; make sure it is initialized
				if this.previous == cs.rcRoot && scroll == 0 {
					if _, has := cs.uninitializedGlobals[name]; has {
						panic(fmt.Sprintf("scope.reach(\"%v\") -> unitialized symbol!", name))
					}
				}
				return core.Reference{Index: index, Scroll: scroll}
			}
		}
		this = this.previous
	}
	panic(fmt.Sprintf("scope.reach(\"%v\") -> unreachable symbol!", name))
}

func (cs *CompilerState) isInBuiltIn(name string) bool {
	this := cs.rc
	for scroll := 0; this != nil; scroll++ {
		for i := len(this.lookup) - 1; i >= 0; i-- {
			if _, exists := this.lookup[i][name]; exists {
				// if built-in scope
				if this.previous == nil {
					return true
				}

			}
		}
		this = this.previous
	}
	return false
}

/* func (s *compilerState) addToCaptured(ref core.Reference) (index int) {
	// find most recent fn
	maxIp := -1
	for ip := range s.fns {
		if ip > maxIp {
			maxIp = ip
		}
	}

	info := s.fns[maxIp]
	for _, v := range info.Refs {
		if v.Scroll == ref.Scroll && v.Index == ref.Index {
			return
		}
	}

	info.Refs = append(info.Refs, ref)
	return len(info.Refs) - 1
} */

func (cs *CompilerState) addToCaptured(ref core.Reference) (index int) {
	// find most recent fn
	fn := cs.openFunctions[len(cs.openFunctions)-1]

	accessingGlobal := len(cs.openFunctions) == ref.Scroll
	if !accessingGlobal {
		// owner of variable needs to know that we capture its variable too
		owner := cs.openFunctions[len(cs.openFunctions)-1-ref.Scroll]
		if _, exists := owner.escapedLocals[ref.Index]; !exists {
			owner.escapedLocals[ref.Index] = struct{}{}
		}
	}

	ourInfo := cs.fns[fn.ip]
	for i, v := range ourInfo.Refs {
		if v.Scroll == ref.Scroll && v.Index == ref.Index {
			return i
		}
	}

	ourInfo.Refs = append(ourInfo.Refs, ref)
	return len(ourInfo.Refs) - 1
}

func (cs *CompilerState) addReferenceNameFor(pos int, name string) {
	cs.symbols[pos] = name
}

func (cs *CompilerState) addU16OffsetFor(pos int, offset uint16) {
	cs.output[pos+1] = byte(offset)
	cs.output[pos+2] = byte(offset >> 8)
}

func (cs *CompilerState) addU8OffsetFor(pos int, offset byte) {
	cs.output[pos+1] = offset
}

func (cs *CompilerState) getFnInfoFor(pos int) *core.FuncInfo {
	if info, exists := cs.fns[pos]; exists {
		return info
	}
	info := new(core.FuncInfo)
	cs.fns[pos] = info
	return info
}

func (cs *CompilerState) openFunction(pos int) {
	cs.openFunctions = append(cs.openFunctions, struct {
		ip            int
		escapedLocals map[int]struct{}
	}{pos, map[int]struct{}{}})
}

func (cs *CompilerState) closeFunction() {
	fn := cs.openFunctions[len(cs.openFunctions)-1]
	cs.openFunctions = cs.openFunctions[:len(cs.openFunctions)-1]

	funcInfo := cs.fns[fn.ip]

	for index := range funcInfo.Capacity {
		if _, exists := fn.escapedLocals[index]; !exists {
			funcInfo.NonEscaping = append(funcInfo.NonEscaping, index)
		}
	}
}
