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
	top := sc.blocks[len(sc.blocks)-1]
	if _, exists := top[name]; exists {
		return 0, false
	}
	top[name] = Binding{Index: sc.index, IsStatic: isStatic}
	if sc.TrackSlots {
		sc.slots = append(sc.slots, SlotInfo{Name: name, IsStatic: isStatic})
	}
	sc.index++
	return sc.index - 1, true
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
