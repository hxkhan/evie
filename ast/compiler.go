package ast

import (
	"fmt"
	"strings"

	"github.com/hk-32/evie/core"
)

func NewVM(exports map[string]core.Value, optimise bool) *Machine {
	vm := &Machine{
		globals:              make(map[string]int),
		rcRoot:               &reachability{[]map[string]int{make(map[string]int, len(exports))}, 0, 0, nil},
		uninitializedGlobals: make(map[string]struct{}),
		optimise:             optimise,
	}
	vm.rc = vm.rcRoot

	builtins := make([]core.Value, len(exports))
	for name, value := range exports {
		builtins[vm.declare(name)] = value
	}
	// extend from builtin to global scope
	vm.scopeExtend()

	vm.Machine = core.NewMachine(builtins)
	return vm
}

func (vm *Machine) Run(node Node) (core.Value, error) {
	code := node.compile(vm)

	vm.AcquireGIL()
	defer vm.ReleaseGIL()

	// ensure that the globals are large enough
	if len(vm.Globals) < len(vm.globals) {
		for range len(vm.globals) - len(vm.Globals) {
			vm.Globals = append(vm.Globals, vm.Boxes.New())
		}
	}

	// fetch a coroutine and prepare it
	rt := vm.Coroutines.New()
	rt.Stack = vm.Globals
	rt.Basis = []int{0}

	// run code
	v, err := code(rt)
	// free coroutine
	vm.Coroutines.Put(rt)
	// check errors
	if err == core.ErrReturnSignal {
		return v, nil
	}
	return core.Value{}, nil
}

func (vm *Machine) GetGlobal(name string) *core.Value {
	if index, exists := vm.globals[name]; exists {
		return vm.Globals[index]
	}
	return nil
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
func (p Package) compile(vm *Machine) core.Instruction {
	var code []core.Instruction

	// 1. declare all symbols
	for _, node := range p.Code {
		if fnDec, isFnDecl := node.(Fn); isFnDecl {
			vm.globals[fnDec.Name] = vm.declare(fnDec.Name)
		}

		if iGet, isIdentDec := node.(IdentDec); isIdentDec {
			vm.uninitializedGlobals[iGet.Name] = struct{}{}
			vm.globals[iGet.Name] = vm.declare(iGet.Name)
		}
	}

	// 2. physically move function initialization to the top
	for _, node := range p.Code {
		if fnDec, isFnDecl := node.(Fn); isFnDecl {
			in := fnDec.compileInGlobal(vm)
			code = append(code, in)
		}
	}

	// compile the rest of the code
	for _, node := range p.Code {
		if _, isFnDecl := node.(Fn); isFnDecl {
			continue
		}

		// compile global variable declarations in a special way
		if iDec, isIdentDec := node.(IdentDec); isIdentDec {
			in := iDec.compileInGlobal(vm)
			delete(vm.uninitializedGlobals, iDec.Name)
			code = append(code, in)
			continue
		}

		// other code
		in := node.compile(vm)
		code = append(code, in)
	}

	return func(rt *core.CoRoutine) (core.Value, error) {
		for _, in := range code {
			if v, err := in(rt); err != nil {
				return v, err
			}
		}
		return core.Value{}, nil
	}
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

type Machine struct {
	core.Machine // embed runtime machine

	globals                    map[string]int
	openFunctionsRefs          [][]core.Reference // open functions and their captured variables
	openFunctionsEscapedLocals []map[int]struct{} // open functions and their escapee locals

	rc                   *reachability // current scope
	rcRoot               *reachability // built-in scope
	uninitializedGlobals map[string]struct{}
	optimise             bool
}

func (vm *Machine) scopeExtend() {
	vm.rc = &reachability{lookup: []map[string]int{{}}, previous: vm.rc}
	//fmt.Println("AFTER scopeExtend():", s.rc)
}

func (vm *Machine) scopeDeExtend() {
	vm.rc = vm.rc.previous
	//fmt.Println("AFTER scopeDeExtend():", s.rc)
}

func (vm *Machine) scopeCapacity() int {
	return max(vm.rc.index, vm.rc.cap)
}

func (vm *Machine) scopeOpenBlock() {
	vm.rc.lookup = append(vm.rc.lookup, map[string]int{})
}

func (vm *Machine) scopeCloseBlock() {
	vm.rc.lookup = vm.rc.lookup[:len(vm.rc.lookup)-1]
}

func (vm *Machine) scopeReuseBlock() {
	top := vm.rc.lookup[len(vm.rc.lookup)-1]
	// save current cap, might be bigger than the reused cap later; in that case, we want the biggest
	if vm.rc.cap < vm.rc.index {
		vm.rc.cap = vm.rc.index
	}
	vm.rc.index -= len(top)
	for k := range top {
		delete(top, k)
	}
}

func (vm *Machine) declare(name string) (index int) {
	scope := vm.rc.lookup[len(vm.rc.lookup)-1]
	if _, exists := scope[name]; exists {
		panic(fmt.Sprintf("declare(\"%v\") -> double declaration of symbol!", name))
	}
	scope[name] = vm.rc.index
	vm.rc.index++
	return vm.rc.index - 1
}

// like reach but it has to be already declared locally
func (vm *Machine) get(name string) (index int) {
	scope := vm.rc.lookup[len(vm.rc.lookup)-1]
	if i, exists := scope[name]; exists {
		return i
	}
	panic("get() -> why is it not declared already?")
}

func (vm *Machine) reach(name string) core.Reference {
	this := vm.rc
	for scroll := 0; this != nil; scroll++ {
		for i := len(this.lookup) - 1; i >= 0; i-- {
			if index, exists := this.lookup[i][name]; exists {
				// if built-in scope then return scroll -1 to signal that
				if this.previous == nil {
					return core.Reference{Index: index, Scroll: -1}
				}
				// if accessing global from global; make sure it is initialized
				if this.previous == vm.rcRoot && scroll == 0 {
					if _, has := vm.uninitializedGlobals[name]; has {
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

func (vm *Machine) isInBuiltIn(name string) bool {
	this := vm.rc
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

func (vm *Machine) addToCaptured(ref core.Reference) (index int) {
	accessingGlobal := len(vm.openFunctionsEscapedLocals) == ref.Scroll
	if !accessingGlobal {
		// owner of variable needs to know that its local has escaped
		escapedLocals := vm.openFunctionsEscapedLocals[len(vm.openFunctionsEscapedLocals)-1-ref.Scroll]
		escapedLocals[ref.Index] = struct{}{}
	}

	// capture the ref if not already captured
	ourRefs := vm.openFunctionsRefs[len(vm.openFunctionsRefs)-1]
	for i, theRef := range ourRefs {
		if theRef == ref {
			return i
		}
	}
	ourRefs = append(ourRefs, ref)
	vm.openFunctionsRefs[len(vm.openFunctionsRefs)-1] = ourRefs
	return len(ourRefs) - 1
}

func (vm *Machine) openFunction() {
	vm.openFunctionsRefs = append(vm.openFunctionsRefs, nil)
	vm.openFunctionsEscapedLocals = append(vm.openFunctionsEscapedLocals, map[int]struct{}{})
}

func (vm *Machine) closeFunction() (refs []core.Reference, escapees map[int]struct{}) {
	// pop top
	refs = vm.openFunctionsRefs[len(vm.openFunctionsRefs)-1]
	vm.openFunctionsRefs = vm.openFunctionsRefs[:len(vm.openFunctionsRefs)-1]

	escapees = vm.openFunctionsEscapedLocals[len(vm.openFunctionsEscapedLocals)-1]
	vm.openFunctionsEscapedLocals = vm.openFunctionsEscapedLocals[:len(vm.openFunctionsEscapedLocals)-1]

	return refs, escapees
}
