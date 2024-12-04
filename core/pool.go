package core

type pool[T any] []*T

func (p *pool[T]) new() *T {
	if len(*p) == 0 {
		return new(T)
	}
	// ugly ass derefs but its fine
	obj := (*p)[len(*p)-1]
	*p = (*p)[:len(*p)-1]
	return obj
}

func (p *pool[T]) put(obj *T) {
	if len(*p) < cap(*p) {
		*p = append(*p, obj)
	}
}
