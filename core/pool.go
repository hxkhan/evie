package core

const poolSize = 48

type pool []*Value

var boxPool pool

func init() {
	boxPool = make(pool, poolSize)
	for i := 0; i < poolSize; i++ {
		boxPool[i] = new(Value)
	}
}

func (p *pool) Get() *Value {
	if len(*p) == 0 {
		return new(Value)
	}
	// ugly ass derefs but its fine
	obj := (*p)[len(*p)-1]
	*p = (*p)[:len(*p)-1]
	return obj
}

func (p *pool) Put(obj *Value) {
	if len(*p) < cap(*p) {
		*p = append(*p, obj)
	}
}
