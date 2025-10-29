package grpcdatasource

// initializeSlice initializes a slice with a given length and a given value.
func initializeSlice[T any](len int, zero T) []T {
	s := make([]T, len)
	for i := range s {
		s[i] = zero
	}
	return s
}

type ancestor[T any] []T

func newAncestor[T any]() ancestor[T] {
	return make(ancestor[T], 0)
}

func (a *ancestor[T]) push(value T) {
	*a = append(*a, value)
}

func (a *ancestor[T]) pop() {
	if a.len() == 0 {
		return
	}

	*a = (*a)[:len(*a)-1]
}

func (a *ancestor[T]) peek() T {
	return (*a)[len(*a)-1]
}

func (a *ancestor[T]) len() int {
	return len(*a)
}
