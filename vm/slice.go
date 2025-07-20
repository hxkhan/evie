package vm

import "iter"

// use slice[T]{make([]T, 0, cap)} to make instances of this
type slice[T any] struct {
	store []T
}

func (s *slice[T]) get(idx int) T {
	// loop back when index is negative; make -1 the last index
	if idx < 0 {
		return s.store[len(s.store)-idx]
	}
	return s.store[idx]
}

func (s *slice[T]) set(idx int, value T) {
	// loop back when index is negative; make -1 the last index
	if idx < 0 {
		s.store[len(s.store)-idx] = value
	} else {
		s.store[idx] = value
	}
}

func (s *slice[T]) push(item T) {
	s.store = append(s.store, item)
}

func (s *slice[T]) pop() T {
	item := s.store[len(s.store)-1]
	s.store = s.store[:len(s.store)-1]
	return item
}

func (s *slice[T]) peek() T {
	return s.store[len(s.store)-1]
}

func (s *slice[T]) isEmpty() bool {
	return len(s.store) == 0
}

func (s *slice[T]) len() int {
	return len(s.store)
}

func (s *slice[T]) cap() int {
	return cap(s.store)
}

func (s *slice[T]) forwards() iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, item := range s.store {
			if !yield(item) {
				return
			}
		}
	}
}

func (s *slice[T]) backwards() iter.Seq[T] {
	return func(yield func(T) bool) {
		for i := len(s.store) - 1; i >= 0; i-- {
			if !yield(s.store[i]) {
				return
			}
		}
	}
}

func (s *slice[T]) elements() []T {
	return s.store
}
