package ds

// Slice is a small stack helper used by the compiler.
type Slice[T any] []T

func (s *Slice[T]) Push(item T) {
	*s = append(*s, item)
}

func (s *Slice[T]) Pop() T {
	last := len(*s) - 1
	item := (*s)[last]
	var zero T
	(*s)[last] = zero
	*s = (*s)[:last]
	return item
}

func (s *Slice[T]) Last(i int) T {
	return (*s)[len(*s)-1-i]
}

func (s *Slice[T]) Len() int {
	return len(*s)
}
