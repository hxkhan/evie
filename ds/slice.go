package ds

import "iter"

// use make(Slice[T], len, cap) or Slice[T]{} to make instances of this
type Slice[T any] []T

func (s *Slice[T]) Get(index int) T {
	// loop back when index is negative
	if index < 0 {
		return (*s)[len((*s))+index]
	}
	return (*s)[index]
}

func (s *Slice[T]) Set(index int, value T) {
	// loop back when index is negative
	if index < 0 {
		(*s)[len((*s))+index] = value
	} else {
		(*s)[index] = value
	}
}

func (s *Slice[T]) Push(item T) {
	(*s) = append((*s), item)
}

func (s *Slice[T]) Pop() T {
	item := (*s)[len((*s))-1]
	(*s) = (*s)[:len((*s))-1]
	return item
}

func (s *Slice[T]) Last() T {
	return (*s)[len((*s))-1]
}

func (s *Slice[T]) IsEmpty() bool {
	return len((*s)) == 0
}

func (s *Slice[T]) Len() int {
	return len((*s))
}

func (s *Slice[T]) Cap() int {
	return cap((*s))
}

func (s *Slice[T]) Forwards() iter.Seq2[int, T] {
	return func(yield func(int, T) bool) {
		for i := 0; i < len((*s)); i++ {
			if !yield(i, (*s)[i]) {
				return
			}
		}
	}
}

func (s *Slice[T]) Backwards() iter.Seq2[int, T] {
	return func(yield func(int, T) bool) {
		for i := len((*s)) - 1; i >= 0; i-- {
			if !yield(i, (*s)[i]) {
				return
			}
		}
	}
}
