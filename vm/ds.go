package vm

// use make(pool[T], 0, cap) to make instances of this
/* type pool[T any] []*T

func (p *pool[T]) Get() (obj *T) {
	if len((*p)) == 0 {
		return new(T)
	}

	obj = (*p)[len((*p))-1]
	(*p) = (*p)[:len((*p))-1]
	return obj
}

func (p *pool[T]) Put(obj *T) {
	if len((*p)) < cap((*p)) {
		(*p) = append((*p), obj)
	}
}

type set[T comparable] map[T]struct{}

func (s set[T]) add(item T) {
	s[item] = struct{}{}
}

func (s set[T]) has(item T) bool {
	_, exists := s[item]
	return exists
}

func (s set[T]) len() int {
	return len(s)
}

// use make(slice[T], len, cap) or slice[T]{} to make instances of this
type slice[T any] []T

func (s *slice[T]) get(idx int) T {
	// loop back when index is negative
	if idx < 0 {
		return (*s)[len((*s))+idx]
	}
	return (*s)[idx]
}

func (s *slice[T]) set(idx int, value T) {
	// loop back when index is negative
	if idx < 0 {
		(*s)[len((*s))+idx] = value
	} else {
		(*s)[idx] = value
	}
}

func (s *slice[T]) append(item T) {
	(*s) = append((*s), item)
}

func (s *slice[T]) pop() T {
	item := (*s)[len((*s))-1]
	(*s) = (*s)[:len((*s))-1]
	return item
}

func (s *slice[T]) last() T {
	return (*s)[len((*s))-1]
}

func (s *slice[T]) isEmpty() bool {
	return len((*s)) == 0
}

func (s *slice[T]) len() int {
	return len((*s))
}

func (s *slice[T]) cap() int {
	return cap((*s))
}

func (s *slice[T]) forwards() iter.Seq2[int, T] {
	return func(yield func(int, T) bool) {
		for i := 0; i < len((*s)); i++ {
			if !yield(i, (*s)[i]) {
				return
			}
		}
	}
}

func (s *slice[T]) backwards() iter.Seq2[int, T] {
	return func(yield func(int, T) bool) {
		for i := len((*s)) - 1; i >= 0; i-- {
			if !yield(i, (*s)[i]) {
				return
			}
		}
	}
}
*/
