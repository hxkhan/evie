package pool

type Instance[T any] struct {
	store []*T
}

func Make[T any](cap int) Instance[T] {
	return Instance[T]{store: make([]*T, 0, cap)}
}

func (this *Instance[T]) Get() *T {
	if len(this.store) == 0 {
		return new(T)
	}

	obj := this.store[len(this.store)-1]
	this.store = this.store[:len(this.store)-1]
	return obj
}

func (this *Instance[T]) Put(obj *T) {
	if len(this.store) < cap(this.store) {
		this.store = append(this.store, obj)
	}
}
