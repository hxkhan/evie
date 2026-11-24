package vm

import (
	"hash/maphash"
	"iter"
	"slices"
	"sort"
	"unsafe"
)

// Object is the storage of an evie object: named fields in the order they were
// added. Keys and values sit in order in two slices. A small object finds a
// key by comparing them in turn, which beats hashing for a handful of fields;
// the objects one literal makes share their keys until one of them gets a new
// key or loses one.
//
// An object with more than maxLinearKeys keys also keeps a hash table of
// slots, the way CPython's dict does (see hashIndex). A lookup or an insert
// hashes the key once and probes the table; growing it reuses the stored
// hashes; and the order stays the order of the slices. The table hangs off a
// pointer so that the many small objects stay small.
type Object struct {
	keys   []string
	values []Value
	big    *hashIndex // nil for a small object
	shared bool       // keys and big belong to a literal and must be copied before they change
}

// hashIndex finds the slot of a key in a big object: table holds, for each
// bucket, the slot of a key plus one (zero for an empty bucket), and hashes
// holds the hash of each key by slot
type hashIndex struct {
	table  []int32
	hashes []uint32
}

// maxLinearKeys is the most keys an object finds by comparing them in turn
const maxLinearKeys = 8

// keySeed seeds the hashes of keys. It is random per process, which changes
// where keys land in a table but never the order of an object.
var keySeed = maphash.MakeSeed()

func hashKey(key string) uint32 {
	h := maphash.String(keySeed, key)
	return uint32(h ^ h>>32)
}

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

// newShaped makes an object with the keys of a literal, which it shares along
// with their table, and zero values for them; the literal's code fills them in
func newShaped(keys []string, big *hashIndex) *Object {
	return &Object{keys: keys, values: make([]Value, len(keys)), big: big, shared: true}
}

// indexOf builds the hash table of keys, or nil for few enough keys to compare
func indexOf(keys []string) *hashIndex {
	if len(keys) <= maxLinearKeys {
		return nil
	}
	hashes := make([]uint32, len(keys), cap(keys))
	for i, k := range keys {
		hashes[i] = hashKey(k)
	}
	return &hashIndex{table: buildTable(hashes, tableSize(len(keys))), hashes: hashes}
}

// tableSize is the number of buckets for n keys: a power of two at least
// twice n, so that probes stay short
func tableSize(n int) int {
	size := 16
	for size < 2*n {
		size *= 2
	}
	return size
}

// buildTable places every slot of hashes in a new table of size buckets
func buildTable(hashes []uint32, size int) []int32 {
	table := make([]int32, size)
	mask := uint32(size - 1)
	for slot, h := range hashes {
		i := h & mask
		for table[i] != 0 {
			i = (i + 1) & mask
		}
		table[i] = int32(slot + 1)
	}
	return table
}

// slot is where key lives, or -1
func (o *Object) slot(key string) int {
	if o.big != nil {
		slot, _, _ := o.big.probe(o.keys, key, hashKey(key))
		return slot
	}
	for i, k := range o.keys {
		if sameKey(k, key) {
			return i
		}
	}
	return -1
}

// probe finds key, whose hash is h, among keys: its slot, or -1 and the
// empty bucket where it would go
func (x *hashIndex) probe(keys []string, key string, h uint32) (slot int, bucket uint32, found bool) {
	table, hashes := x.table, x.hashes
	mask := uint32(len(table) - 1)
	for i := h & mask; ; i = (i + 1) & mask {
		s := table[i]
		if s == 0 {
			return -1, i, false
		}
		if hashes[s-1] == h && sameKey(keys[s-1], key) {
			return int(s - 1), i, true
		}
	}
}

// insert places a new slot whose key's hash is h, growing the table when it
// would get more than half full
func (x *hashIndex) insert(slot int, h uint32) {
	x.hashes = append(x.hashes, h)
	if 2*len(x.hashes) > len(x.table) {
		x.table = buildTable(x.hashes, tableSize(len(x.hashes)))
		return
	}
	mask := uint32(len(x.table) - 1)
	i := h & mask
	for x.table[i] != 0 {
		i = (i + 1) & mask
	}
	x.table[i] = int32(slot + 1)
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

// Set sets key to v, adding it after the others when it is new. A big object
// hashes key once, whether it finds it or adds it.
func (o *Object) Set(key string, v Value) {
	x := o.big
	if x == nil {
		if i := o.slot(key); i >= 0 {
			o.values[i] = v
			return
		}
		o.add(key, v)
		return
	}
	h := hashKey(key)
	slot, bucket, found := x.probe(o.keys, key, h)
	if found {
		o.values[slot] = v
		return
	}
	if o.shared || 2*(len(o.keys)+1) > len(x.table) {
		o.addHashed(key, v, h) // copies or grows the table, so the bucket moves
		return
	}
	o.keys = append(o.keys, key)
	o.values = append(o.values, v)
	x.hashes = append(x.hashes, h)
	x.table[bucket] = int32(len(o.keys))
}

// add appends a key the object does not have
func (o *Object) add(key string, v Value) {
	if o.big != nil {
		o.addHashed(key, v, hashKey(key))
		return
	}
	o.unshare()
	o.keys = append(o.keys, key)
	o.values = append(o.values, v)
	if len(o.keys) > maxLinearKeys {
		o.big = indexOf(o.keys)
	}
}

// addHashed appends a key the object does not have to a big object, given its hash
func (o *Object) addHashed(key string, v Value, h uint32) {
	o.unshare()
	o.keys = append(o.keys, key)
	o.values = append(o.values, v)
	o.big.insert(len(o.keys)-1, h)
}

// unshare copies what the object shares with its literal before it changes
func (o *Object) unshare() {
	if !o.shared {
		return
	}
	o.keys = append(make([]string, 0, len(o.keys)+4), o.keys...)
	if o.big != nil {
		o.big = &hashIndex{table: slices.Clone(o.big.table), hashes: append(make([]uint32, 0, len(o.big.hashes)+4), o.big.hashes...)}
	}
	o.shared = false
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
	o.values = slices.Delete(o.values, i, i+1)
	if o.big != nil {
		hashes := slices.Delete(slices.Clone(o.big.hashes), i, i+1)
		o.big = nil
		if len(o.keys) > maxLinearKeys {
			o.big = &hashIndex{table: buildTable(hashes, tableSize(len(hashes))), hashes: hashes}
		}
	}
	o.shared = false
	return true
}

// Clear removes every key
func (o *Object) Clear() {
	o.keys, o.values, o.big, o.shared = nil, nil, nil, false
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
