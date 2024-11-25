package core

import "github.com/hk-32/evie/box"

const poolSize = 48

type pool []*box.Value

var boxPool pool

func init() {
	boxPool = make(pool, poolSize)
	for i := 0; i < poolSize; i++ {
		boxPool[i] = new(box.Value)
	}
}

func (p *pool) Get() *box.Value {
	if len(*p) == 0 {
		return new(box.Value)
	}
	// ugly ass derefs but its fine
	obj := (*p)[len(*p)-1]
	*p = (*p)[:len(*p)-1]
	return obj
}

func (p *pool) Put(obj *box.Value) {
	if len(*p) < cap(*p) {
		*p = append(*p, obj)
	}
}
