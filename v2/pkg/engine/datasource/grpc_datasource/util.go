package grpcdatasource

import "github.com/wundergraph/graphql-go-tools/v2/pkg/ast"

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

// inlineFragmentRefFromAncestors returns the inline fragment ref for the field
// at the top of the walker's ancestor stack, or ast.InvalidRef if the field is
// not a direct child of an inline fragment.
//
// When entering a field's selection set, the walker Ancestors slice has the shape:
//
//	[..., (maybe inline fragment), parent selection set, field]
//
// Ancestors[-3] is therefore the node that directly contains the parent selection
// set — an inline fragment if and only if the field is a direct child of one.
func inlineFragmentRefFromAncestors(ancestors []ast.Node) int {
	if len(ancestors) < 3 {
		return ast.InvalidRef
	}
	ancestor := ancestors[len(ancestors)-3]
	if ancestor.Kind != ast.NodeKindInlineFragment {
		return ast.InvalidRef
	}
	return ancestor.Ref
}
