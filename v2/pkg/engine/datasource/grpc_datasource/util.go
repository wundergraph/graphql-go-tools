package grpcdatasource

// initializeSlice initializes a slice with a given length and a given value.
func initializeSlice[T any](len int, zero T) []T {
	s := make([]T, len)
	for i := range s {
		s[i] = zero
	}
	return s
}

// stack is a generic LIFO (Last In First Out) data structure that stores elements of type T.
type stack[T any] []T

// newStack creates and returns a new empty stack for elements of type T.
func newStack[T any](size int) stack[T] {
	return make(stack[T], 0, size)
}

// push adds a new element to the top of the stack.
func (a *stack[T]) push(value T) {
	*a = append(*a, value)
}

// pop removes the top element from the stack.
// If the stack is empty, this operation is a no-op.
func (a *stack[T]) pop() {
	if a.len() == 0 {
		return
	}

	*a = (*a)[:len(*a)-1]
}

// peek returns the top element of the stack without removing it.
// Note: This function will panic if called on an empty stack.
func (a *stack[T]) peek() T {
	return (*a)[len(*a)-1]
}

// len returns the number of elements currently in the stack.
func (a *stack[T]) len() int {
	return len(*a)
}

// capacity returns the capacity of the stack.
func (a *stack[T]) capacity() int {
	return cap(*a)
}
