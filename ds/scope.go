package ds

import (
	"fmt"
	"strings"
)

type bindings map[string]int

type Scope struct {
	blocks []bindings // each block-scope gets its own bindings table
	index  int        // current free index in this whole scope
	tmp    int        // the biggest value that index has ever gotten
}

func (sc *Scope) Capacity() int {
	return max(sc.index, sc.tmp)
}

func (sc *Scope) OpenBlock() {
	sc.blocks = append(sc.blocks, bindings{})
}

func (sc *Scope) CloseBlock() {
	sc.blocks = sc.blocks[:len(sc.blocks)-1]
	// NOTE: Future optimisation -> we can decrement the index if nothing in the block escapes
}

func (sc *Scope) ReuseBlock() {
	top := sc.blocks[len(sc.blocks)-1]
	// save current index
	// the alternative block might require less capacity
	// in that case, we want to accomodate for the biggest
	sc.tmp = max(sc.tmp, sc.index)
	sc.index -= len(top)
	for k := range top {
		delete(top, k)
	}
}

// Declare adds a new binding to the current block-scope
func (sc *Scope) Declare(name string) (index int, success bool) {
	top := sc.blocks[len(sc.blocks)-1]
	if _, exists := top[name]; exists {
		return 0, false
	}
	top[name] = sc.index
	sc.index++
	return sc.index - 1, true
}

// Reach searches for a binding in this scope
func (sc *Scope) Reach(name string) (index int, success bool) {
	for bi := len(sc.blocks) - 1; bi >= 0; bi-- {
		if index, exists := sc.blocks[bi][name]; exists {
			return index, true
		}
	}
	return 0, false
}

func (sc Scope) String() string {
	s := strings.Builder{}
	s.WriteByte('{')

	for _, lookup := range sc.blocks {
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
	return s.String()
}
