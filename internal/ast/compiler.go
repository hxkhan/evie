package ast

import (
	"fmt"
	"strings"
	"unsafe"

	"github.com/hk-32/evie/core"
	"github.com/hk-32/evie/internal/op"
)

func NewCompiler(optimise bool, exports map[string]core.Value) *CompilerState {
	cs := &CompilerState{
		globals:              make(map[string]*core.Value),
		fns:                  make(map[int]*core.FuncInfo),
		symbols:              make(map[int]string),
		rc:                   &reachability{[]map[string]int{make(map[string]int, len(exports))}, 0, 0, nil},
		uninitializedGlobals: make(map[string]struct{}),
		optimise:             optimise,
	}

	cs.builtins = make([]core.Value, len(exports))
	for name, value := range exports {
		cs.builtins[cs.declare(name)] = value
	}
	cs.scopeExtend()

	return cs
}

func (cs *CompilerState) Compile(node Node) (*core.CoRoutine, error) {
	node.compile(cs)
	return core.SetProgram(cs.output, cs.globals, cs.builtins, cs.globalScope, cs.symbols, cs.fns)
}

func (cs *CompilerState) BuiltIns() []core.Value {
	return cs.builtins
}

func (cs *CompilerState) Globals() []*core.Value {
	return cs.globalScope
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

// bit of special rules with hoisting
/* Process:
All declarations are physically shifted to the top

So this is not possible because the order is maintained:
	var x = y + 2
	var y = 10

But this is; becuase the declaration ends up being shifted to the top:
	println(x)
	var x = 10
*/
func (p Package) compile(cs *CompilerState) int {
	// 1. declare all symbols
	for _, node := range p.Code {
		if fnDec, isFnDecl := node.(Fn); isFnDecl {
			cs.declare(fnDec.Name)
			v := new(core.Value)
			cs.globals[fnDec.Name] = v
			cs.globalScope = append(cs.globalScope, v)
		}

		if iGet, isIdentDec := node.(IdentDec); isIdentDec {
			cs.uninitializedGlobals[iGet.Name] = struct{}{}
			cs.declare(iGet.Name)
			v := new(core.Value)
			cs.globals[iGet.Name] = v
			cs.globalScope = append(cs.globalScope, v)
		}
	}

	// 2. physically move function declarations to the top
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
	output      []byte
	globals     map[string]*core.Value
	globalScope []*core.Value

	builtins      []core.Value
	fns           map[int]*core.FuncInfo
	symbols       map[int]string
	openFunctions []struct {
		ip            int
		escapedLocals map[int]struct{}
	} // open functions and their escape'e locals
	rc                   *reachability
	uninitializedGlobals map[string]struct{}
	optimise             bool
}

func (s *CompilerState) emit(bytes ...byte) (pos int) {
	pos = len(s.output)
	s.output = append(s.output, bytes...)
	return pos
}

func (s *CompilerState) set(ip int, b byte) {
	s.output[ip] = b
}

func (s *CompilerState) emitString(str string) (pos int) {
	pos = len(s.output)
	size := uint16(len(str))
	s.output = append(s.output, op.STR)
	s.output = append(s.output, byte(size))
	s.output = append(s.output, byte(size>>8))
	s.output = append(s.output, str...)
	return pos
}

func (s *CompilerState) emitInt64(n int64) (pos int) {
	pos = len(s.output)
	s.output = append(s.output, op.INT)
	s.output = append(s.output, (*[8]byte)(unsafe.Pointer(&n))[:]...)
	return pos
}

func (s *CompilerState) emitFloat64(n float64) (pos int) {
	pos = len(s.output)
	s.output = append(s.output, op.FLOAT)
	s.output = append(s.output, (*[8]byte)(unsafe.Pointer(&n))[:]...)
	return pos
}

func (s *CompilerState) len() int {
	return len(s.output)
}

func (s *CompilerState) scopeExtend() {
	s.rc = &reachability{lookup: []map[string]int{{}}, previous: s.rc}
	//fmt.Println("AFTER scopeExtend():", s.rc)
}

func (s *CompilerState) scopeDeExtend() {
	s.rc = s.rc.previous
	//fmt.Println("AFTER scopeDeExtend():", s.rc)
}

func (s *CompilerState) scopeCapacity() int {
	return max(s.rc.index, s.rc.cap)
}

func (s *CompilerState) scopeOpenBlock() {
	s.rc.lookup = append(s.rc.lookup, map[string]int{})
}

func (s *CompilerState) scopeCloseBlock() {
	s.rc.lookup = s.rc.lookup[:len(s.rc.lookup)-1]
}

func (s *CompilerState) scopeReuseBlock() {
	top := s.rc.lookup[len(s.rc.lookup)-1]
	// save current cap, might be bigger than the reused cap later; in that case, we want the biggest
	if s.rc.cap < s.rc.index {
		s.rc.cap = s.rc.index
	}
	s.rc.index -= len(top)
	for k := range top {
		delete(top, k)
	}
}

func (s *CompilerState) declare(name string) (index int) {
	scope := s.rc.lookup[len(s.rc.lookup)-1]
	if _, exists := scope[name]; exists {
		panic(fmt.Sprintf("declare(\"%v\") -> double declaration of symbol!", name))
	}
	scope[name] = s.rc.index
	s.rc.index++
	return s.rc.index - 1
}

// like reach but it has to be already declared alr locally
func (s *CompilerState) get(name string) (index int) {
	scope := s.rc.lookup[len(s.rc.lookup)-1]
	if i, exists := scope[name]; exists {
		return i
	}
	panic("get() -> why is it not declared already?")
}

func (s *CompilerState) reach(name string) core.Reference {
	this := s.rc
	for scroll := 0; this != nil; scroll++ {
		for i := len(this.lookup) - 1; i >= 0; i-- {
			if index, exists := this.lookup[i][name]; exists {
				// if built-in scope then return scroll -1 to signal that
				if this.previous == nil {
					return core.Reference{Index: index, Scroll: -1}
				}
				// if accessing global from global; make sure it is initialized
				if this.previous != nil && this.previous.previous == nil && scroll == 0 {
					if _, has := s.uninitializedGlobals[name]; has {
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

func (s *CompilerState) isInBuiltIn(name string) bool {
	this := s.rc
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

func (s *CompilerState) addToCaptured(ref core.Reference) (index int) {
	// find most recent fn
	fn := s.openFunctions[len(s.openFunctions)-1]

	accessingGlobal := len(s.openFunctions) == ref.Scroll
	if !accessingGlobal {
		// owner of variable needs to know that we capture its variable too
		owner := s.openFunctions[len(s.openFunctions)-1-ref.Scroll]
		if _, exists := owner.escapedLocals[ref.Index]; !exists {
			owner.escapedLocals[ref.Index] = struct{}{}
		}
	}

	ourInfo := s.fns[fn.ip]
	for i, v := range ourInfo.Refs {
		if v.Scroll == ref.Scroll && v.Index == ref.Index {
			return i
		}
	}

	ourInfo.Refs = append(ourInfo.Refs, ref)
	return len(ourInfo.Refs) - 1
}

func (s *CompilerState) addReferenceNameFor(pos int, name string) {
	s.symbols[pos] = name
}

func (s *CompilerState) addU16OffsetFor(pos int, offset uint16) {
	s.output[pos+1] = byte(offset)
	s.output[pos+2] = byte(offset >> 8)
}

func (s *CompilerState) addU8OffsetFor(pos int, offset byte) {
	s.output[pos+1] = offset
}

func (s *CompilerState) getFnInfoFor(pos int) *core.FuncInfo {
	if info, exists := s.fns[pos]; exists {
		return info
	}
	info := new(core.FuncInfo)
	s.fns[pos] = info
	return info
}

func (s *CompilerState) openFunction(pos int) {
	s.openFunctions = append(s.openFunctions, struct {
		ip            int
		escapedLocals map[int]struct{}
	}{pos, map[int]struct{}{}})
}

func (s *CompilerState) closeFunction() {
	fn := s.openFunctions[len(s.openFunctions)-1]
	s.openFunctions = s.openFunctions[:len(s.openFunctions)-1]

	funcInfo := s.fns[fn.ip]

	for index := range funcInfo.Capacity {
		if _, exists := fn.escapedLocals[index]; !exists {
			funcInfo.NonEscaping = append(funcInfo.NonEscaping, index)
		}
	}
}
