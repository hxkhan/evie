package vm

import (
	"iter"
	"slices"
	"sort"
	"unsafe"
)

// Object is the storage of an evie object: named fields in the order they were
// added. A small object keeps its keys in a slice and finds one by comparing
// them in turn, which beats hashing for a handful of fields; the objects one
// literal makes share that slice until one of them gets a new key or loses one.
// An object with more than maxLinearKeys keys also keeps an index.
type Object struct {
	keys   []string
	values []Value
	index  map[string]int // key -> slot, for an object with many keys
	shared bool           // keys belongs to a literal and must be copied before it changes
}

// maxLinearKeys is the most keys an object finds by comparing them in turn
const maxLinearKeys = 8

// NewObject makes an empty object with room for capacity fields
func NewObject(capacity int) *Object {
	return &Object{keys: make([]string, 0, capacity), values: make([]Value, 0, capacity)}
}

// ObjectOf makes an object holding the entries of m, in the order of their keys
func ObjectOf(m map[string]Value) *Object {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	o := NewObject(len(keys))
	for _, k := range keys {
		o.add(k, m[k])
	}
	return o
}

// newShaped makes an object with the keys of a literal, which it shares, and
// zero values for them; the literal's code fills them in
func newShaped(keys []string, index map[string]int) *Object {
	return &Object{keys: keys, values: make([]Value, len(keys)), index: index, shared: true}
}

// slot is where key lives, or -1
func (o *Object) slot(key string) int {
	if o.index != nil {
		if i, ok := o.index[key]; ok {
			return i
		}
		return -1
	}
	for i, k := range o.keys {
		if sameKey(k, key) {
			return i
		}
	}
	return -1
}

// sameKey is k == key, deciding without comparing bytes when both are the same
// string, as interned names are
func sameKey(k, key string) bool {
	return len(k) == len(key) && (unsafe.StringData(k) == unsafe.StringData(key) || k == key)
}

// find is Get for a field site that remembers in hint where it found key
// last; the site itself checks the hint first, as objects from the same
// literal keep their keys in the same slots, and calls find when it misses
//
//go:noinline
func (o *Object) find(key string, hint *int) (Value, bool) {
	i := o.slot(key)
	if i < 0 {
		return Value{}, false
	}
	*hint = i
	return o.values[i], true
}

// Get returns the value of key
func (o *Object) Get(key string) (Value, bool) {
	if i := o.slot(key); i >= 0 {
		return o.values[i], true
	}
	return Value{}, false
}

// get is Get without the flag: nil for a missing key
func (o *Object) get(key string) Value {
	if i := o.slot(key); i >= 0 {
		return o.values[i]
	}
	return Value{}
}

// Has reports whether the object has key
func (o *Object) Has(key string) bool {
	return o.slot(key) >= 0
}

// Set sets key to v, adding it after the others when it is new
func (o *Object) Set(key string, v Value) {
	if i := o.slot(key); i >= 0 {
		o.values[i] = v
		return
	}
	o.add(key, v)
}

// add appends a key the object does not have
func (o *Object) add(key string, v Value) {
	if o.shared {
		o.keys = append(make([]string, 0, len(o.keys)+4), o.keys...)
		o.shared = false
		if o.index != nil {
			o.index = maps(o.keys)
		}
	}
	o.keys = append(o.keys, key)
	o.values = append(o.values, v)
	switch {
	case o.index != nil:
		o.index[key] = len(o.keys) - 1
	case len(o.keys) > maxLinearKeys:
		o.index = maps(o.keys)
	}
}

// Delete removes key and reports whether the object had it. The remaining
// keys keep their order. It never changes the old key slice in place, so a
// loop over the keys can go on while the object changes.
func (o *Object) Delete(key string) bool {
	i := o.slot(key)
	if i < 0 {
		return false
	}
	o.keys = slices.Delete(slices.Clone(o.keys), i, i+1)
	o.shared = false
	o.values = slices.Delete(o.values, i, i+1)
	if o.index != nil {
		o.index = nil
		if len(o.keys) > maxLinearKeys {
			o.index = maps(o.keys)
		}
	}
	return true
}

// Clear removes every key
func (o *Object) Clear() {
	o.keys, o.values, o.index, o.shared = nil, nil, nil, false
}

// Len is the number of keys
func (o *Object) Len() int {
	return len(o.keys)
}

// Keys returns the keys in order
func (o *Object) Keys() []string {
	return slices.Clone(o.keys)
}

// All iterates over the keys and their values in order. Keys added while it
// runs are not visited and removed ones are skipped.
func (o *Object) All() iter.Seq2[string, Value] {
	return func(yield func(string, Value) bool) {
		keys := o.keys
		for i, k := range keys {
			v, ok := o.at(i, k)
			if !ok {
				continue
			}
			if !yield(k, v) {
				return
			}
		}
	}
}

// at returns the value of key k, which was at slot i when a loop took the keys
func (o *Object) at(i int, k string) (Value, bool) {
	if i < len(o.keys) && o.keys[i] == k {
		return o.values[i], true
	}
	return o.Get(k)
}

// Map copies the object into a Go map
func (o *Object) Map() map[string]Value {
	m := make(map[string]Value, len(o.keys))
	for i, k := range o.keys {
		m[k] = o.values[i]
	}
	return m
}

// maps builds the key -> slot index of keys
func maps(keys []string) map[string]int {
	index := make(map[string]int, len(keys))
	for i, k := range keys {
		index[k] = i
	}
	return index
}
