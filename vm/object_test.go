package vm

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
)

// model is the plainest ordered map, to check Object against
type model struct {
	keys []string
	vals map[string]float64
}

func (m *model) set(k string, v float64) {
	if _, ok := m.vals[k]; !ok {
		m.keys = append(m.keys, k)
	}
	m.vals[k] = v
}

func (m *model) delete(k string) {
	if _, ok := m.vals[k]; ok {
		delete(m.vals, k)
		m.keys = slices.DeleteFunc(m.keys, func(x string) bool { return x == k })
	}
}

// Random sets, deletes and clears, on objects small and big and on objects
// sharing the keys of a literal, leave an Object agreeing with the model on
// every key, value and the order of the keys
func TestObjectAgainstAModel(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	literalKeys := make([]string, 12)
	for i := range literalKeys {
		literalKeys[i] = fmt.Sprintf("k%d", i)
	}
	big := indexOf(literalKeys)

	for round := range 300 {
		var o *Object
		m := &model{vals: map[string]float64{}}
		if round%3 == 0 {
			// an object from a literal with enough keys for a table
			o = newShaped(literalKeys, big)
			for i, k := range literalKeys {
				o.values[i] = BoxNumber(float64(i))
				m.set(k, float64(i))
			}
		} else {
			o = NewObject(0)
		}
		space := 1 + rng.IntN(60) // how many different keys this round uses
		for range 400 {
			k := fmt.Sprintf("k%d", rng.IntN(space))
			switch op := rng.IntN(20); {
			case op < 13:
				v := float64(rng.IntN(1000))
				o.Set(k, BoxNumber(v))
				m.set(k, v)
			case op < 19:
				if o.Delete(k) != (m.vals[k] != 0 || slices.Contains(m.keys, k)) {
					t.Fatalf("round %d: Delete(%q) disagrees", round, k)
				}
				m.delete(k)
			default:
				if rng.IntN(10) == 0 {
					o.Clear()
					m = &model{vals: map[string]float64{}}
				}
			}
			for _, probe := range []string{k, "absent"} {
				got, ok := o.Get(probe)
				want, has := m.vals[probe]
				if ok != has || ok && got.String() != BoxNumber(want).String() {
					t.Fatalf("round %d: Get(%q) = %v %v, want %v %v", round, probe, got, ok, want, has)
				}
			}
		}
		if !slices.Equal(o.Keys(), m.keys) {
			t.Fatalf("round %d: keys %v, want %v", round, o.Keys(), m.keys)
		}
	}

	// the literal's own keys and table are untouched by what its objects did
	for i, k := range literalKeys {
		if k != fmt.Sprintf("k%d", i) {
			t.Fatalf("the literal's keys changed: %v", literalKeys)
		}
	}
	fresh := newShaped(literalKeys, big)
	for i, k := range literalKeys {
		if fresh.slot(k) != i {
			t.Fatalf("the literal's table changed: %q is at %d", k, fresh.slot(k))
		}
	}
}

// a loop over an object sees each key that was there when it began, once,
// in order, while the loop adds and removes keys
func TestObjectIterationWhileChanging(t *testing.T) {
	o := NewObject(0)
	for i := range 20 {
		o.Set(fmt.Sprintf("k%d", i), BoxNumber(float64(i)))
	}
	var seen []string
	for k := range o.All() {
		seen = append(seen, k)
		o.Set(k+"x", BoxNumber(1)) // added keys are not visited
		if k == "k3" {
			o.Delete("k4") // a removed key is skipped
		}
	}
	want := []string{}
	for i := range 20 {
		if i != 4 {
			want = append(want, fmt.Sprintf("k%d", i))
		}
	}
	if !slices.Equal(seen, want) {
		t.Fatalf("saw %v\nwant %v", seen, want)
	}
}
