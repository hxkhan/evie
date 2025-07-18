package scope

import (
	"fmt"
	"iter"
	"strings"
)

type bindings map[string]int

type Instance struct {
	blocks   []bindings // each block-scope gets its own bindings table
	index    int        // current free index in this whole instance
	tmp      int        // the biggest value that index has ever gotten
	previous *Instance  // previous scope instance
}

// New creates a brand new root scope with the initial size specified
func NewScope(initialSize int) *Instance {
	return &Instance{[]bindings{make(bindings, initialSize)}, 0, 0, nil}
}

// New creates an extension of the current scope
func (scope *Instance) New() *Instance {
	return &Instance{blocks: []bindings{{}}, previous: scope}
}

func (scope *Instance) Previous() *Instance {
	return scope.previous
}

func (scope *Instance) Capacity() int {
	return max(scope.index, scope.tmp)
}

func (scope *Instance) OpenBlock() {
	scope.blocks = append(scope.blocks, bindings{})
}

func (scope *Instance) CloseBlock() {
	scope.blocks = scope.blocks[:len(scope.blocks)-1]
	// NOTE: Future optimisation -> we can decrement the index if nothing in the block escapes
}

func (scope *Instance) ReuseBlock() {
	top := scope.blocks[len(scope.blocks)-1]
	// save current index
	// the alternative block might require less capacity
	// in that case, we want to accomodate for the biggest
	scope.tmp = max(scope.tmp, scope.index)
	scope.index -= len(top)
	for k := range top {
		delete(top, k)
	}
}

// Declare adds a new binding to the current block-scope
func (scope *Instance) Declare(name string) (index int, success bool) {
	top := scope.blocks[len(scope.blocks)-1]
	if _, exists := top[name]; exists {
		return 0, false
	}
	top[name] = scope.index
	scope.index++
	return scope.index - 1, true
}

// Reach searches for a binding in this instance
func (scope *Instance) Reach(name string) (index int, success bool) {
	for bi := len(scope.blocks) - 1; bi >= 0; bi-- {
		if index, exists := scope.blocks[bi][name]; exists {
			return index, true
		}
	}
	return 0, false
}

// Instances iterates through instances and yields (instance, scroll)
func (scope *Instance) Instances() iter.Seq2[*Instance, int] {
	return func(yield func(*Instance, int) bool) {
		scroll := 0
		for scope != nil {
			if !yield(scope, scroll) {
				return
			}
			scope = scope.previous
			scroll++
		}
	}
}

func (scope *Instance) String() string {
	s := strings.Builder{}
	s.WriteByte('{')

	for _, lookup := range scope.blocks {
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

	if scope.previous != nil {
		s.WriteString(" -> ")
		s.WriteString(scope.previous.String())
	}
	return s.String()
}
