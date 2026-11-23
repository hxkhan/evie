package ds

import (
	"fmt"
	"strings"
)

type Binding struct {
	Index    int
	IsStatic bool
}

// SlotInfo records the name and mutability of a local stack slot when slot
// tracking is enabled for compiler inspection.
type SlotInfo struct {
	Name     string
	IsStatic bool
}

type bindings map[string]Binding

type Scope struct {
	blocks     []bindings // each block-scope gets its own bindings table
	index      int        // current free index in this whole scope
	TrackSlots bool       // when true, Declare records SlotInfo in slots
	slots      []SlotInfo
}

func (sc *Scope) Capacity() int {
	return sc.index
}

// Slots returns the recorded slot metadata when TrackSlots was enabled.
func (sc *Scope) Slots() []SlotInfo {
	return sc.slots
}

func (sc *Scope) OpenBlock() {
	sc.blocks = append(sc.blocks, bindings{})
}

func (sc *Scope) CloseBlock() {
	sc.blocks = sc.blocks[:len(sc.blocks)-1]
}

// Depth is how many blocks are open
func (sc *Scope) Depth() int {
	return len(sc.blocks)
}

// CloseTo closes blocks until depth are open, as after compiling a statement
// failed halfway
func (sc *Scope) CloseTo(depth int) {
	if depth < len(sc.blocks) {
		sc.blocks = sc.blocks[:depth]
	}
}

// Names lists every binding visible in the scope
func (sc *Scope) Names() []string {
	var names []string
	for _, block := range sc.blocks {
		for name := range block {
			names = append(names, name)
		}
	}
	return names
}

// Declare adds a new binding to the current block-scope
func (sc *Scope) Declare(name string, isStatic bool) (index int, success bool) {
	index, success = sc.Reserve(name, isStatic)
	if success {
		sc.Bind(name, index, isStatic)
	}
	return index, success
}

// Reserve allocates the slot for a declaration in the current block-scope
// without making name visible yet, so code compiled before Bind still sees
// any outer binding of the same name
func (sc *Scope) Reserve(name string, isStatic bool) (index int, success bool) {
	if _, exists := sc.blocks[len(sc.blocks)-1][name]; exists {
		return 0, false
	}
	if sc.TrackSlots {
		sc.slots = append(sc.slots, SlotInfo{Name: name, IsStatic: isStatic})
	}
	sc.index++
	return sc.index - 1, true
}

// Bind makes a slot returned by Reserve visible as name in the current block-scope
func (sc *Scope) Bind(name string, index int, isStatic bool) {
	sc.blocks[len(sc.blocks)-1][name] = Binding{Index: index, IsStatic: isStatic}
}

// Reach searches for a binding in this scope
func (sc *Scope) Reach(name string) (binding Binding, success bool) {
	for bi := len(sc.blocks) - 1; bi >= 0; bi-- {
		if b, exists := sc.blocks[bi][name]; exists {
			return b, true
		}
	}
	return binding, false
}

func (sc Scope) String() string {
	var s strings.Builder
	s.WriteByte('{')
	first := true
	for _, lookup := range sc.blocks {
		for name, index := range lookup {
			if !first {
				s.WriteString(", ")
			}
			first = false
			fmt.Fprintf(&s, "%v: %v", name, index)
		}
	}
	s.WriteByte('}')
	return s.String()
}
