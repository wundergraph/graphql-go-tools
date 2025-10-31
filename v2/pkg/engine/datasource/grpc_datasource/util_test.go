package grpcdatasource

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStack(t *testing.T) {
	t.Run("basic push pop and peek operations", func(t *testing.T) {
		s := newStack[int](10)
		s.push(1)
		s.push(2)
		s.push(3)
		require.Equal(t, 10, s.capacity())
		require.Equal(t, 3, s.len())
		s.pop()
		require.Equal(t, 2, s.len())
		require.Equal(t, 2, s.peek())
		require.Equal(t, 2, s.len())
	})

	t.Run("empty stack", func(t *testing.T) {
		s := newStack[int](5)
		require.Equal(t, 0, s.len())
		require.Equal(t, 5, s.capacity())

		// Pop on empty stack should not panic (documented as no-op)
		s.pop()
		require.Equal(t, 0, s.len())

		// Peek on empty stack should panic (documented behavior)
		require.Panics(t, func() {
			s.peek()
		})
	})

	t.Run("push and pop to empty", func(t *testing.T) {
		s := newStack[int](5)
		s.push(10)
		s.push(20)
		s.push(30)
		require.Equal(t, 3, s.len())
		require.Equal(t, 30, s.peek())

		s.pop()
		require.Equal(t, 2, s.len())
		require.Equal(t, 20, s.peek())

		s.pop()
		require.Equal(t, 1, s.len())
		require.Equal(t, 10, s.peek())

		s.pop()
		require.Equal(t, 0, s.len())

		// After popping all elements, peek should panic
		require.Panics(t, func() {
			s.peek()
		})
	})

	t.Run("push after pop", func(t *testing.T) {
		s := newStack[int](10)
		s.push(1)
		s.push(2)
		s.push(3)
		require.Equal(t, 3, s.len())

		s.pop()
		require.Equal(t, 2, s.len())

		s.push(4)
		require.Equal(t, 3, s.len())
		require.Equal(t, 4, s.peek())
	})

	t.Run("fill to capacity and beyond", func(t *testing.T) {
		s := newStack[int](3)
		s.push(1)
		s.push(2)
		s.push(3)
		require.Equal(t, 3, s.len())
		require.Equal(t, 3, s.capacity())

		// Push beyond initial capacity (should grow)
		s.push(4)
		require.Equal(t, 4, s.len())
		require.Greater(t, s.capacity(), 3)
		require.Equal(t, 4, s.peek())
	})

	t.Run("LIFO order verification", func(t *testing.T) {
		s := newStack[int](10)
		for i := 1; i <= 5; i++ {
			s.push(i)
		}

		// Verify LIFO order
		require.Equal(t, 5, s.peek())
		s.pop()
		require.Equal(t, 4, s.peek())
		s.pop()
		require.Equal(t, 3, s.peek())
		s.pop()
		require.Equal(t, 2, s.peek())
		s.pop()
		require.Equal(t, 1, s.peek())
		s.pop()
		require.Equal(t, 0, s.len())
	})

	t.Run("peek does not modify stack", func(t *testing.T) {
		s := newStack[int](5)
		s.push(100)
		s.push(200)

		// Multiple peeks should return same value and not change length
		for i := 0; i < 5; i++ {
			require.Equal(t, 200, s.peek())
			require.Equal(t, 2, s.len())
		}
	})

	t.Run("stack with string type", func(t *testing.T) {
		s := newStack[string](5)
		s.push("hello")
		s.push("world")
		s.push("test")

		require.Equal(t, 3, s.len())
		require.Equal(t, "test", s.peek())

		s.pop()
		require.Equal(t, "world", s.peek())

		s.pop()
		require.Equal(t, "hello", s.peek())
	})

	t.Run("stack with struct type", func(t *testing.T) {
		type testStruct struct {
			id   int
			name string
		}

		s := newStack[testStruct](5)
		s.push(testStruct{id: 1, name: "first"})
		s.push(testStruct{id: 2, name: "second"})

		require.Equal(t, 2, s.len())
		top := s.peek()
		require.Equal(t, 2, top.id)
		require.Equal(t, "second", top.name)

		s.pop()
		top = s.peek()
		require.Equal(t, 1, top.id)
		require.Equal(t, "first", top.name)
	})

	t.Run("large number of operations", func(t *testing.T) {
		s := newStack[int](10)

		// Push 100 items
		for i := 0; i < 100; i++ {
			s.push(i)
		}
		require.Equal(t, 100, s.len())
		require.Equal(t, 99, s.peek())

		// Pop 50 items
		for i := 0; i < 50; i++ {
			s.pop()
		}
		require.Equal(t, 50, s.len())
		require.Equal(t, 49, s.peek())

		// Push 25 more items
		for i := 100; i < 125; i++ {
			s.push(i)
		}
		require.Equal(t, 75, s.len())
		require.Equal(t, 124, s.peek())
	})

	t.Run("alternating push and pop", func(t *testing.T) {
		s := newStack[int](5)

		s.push(1)
		require.Equal(t, 1, s.len())

		s.pop()
		require.Equal(t, 0, s.len())

		s.push(2)
		s.push(3)
		require.Equal(t, 2, s.len())

		s.pop()
		require.Equal(t, 1, s.len())
		require.Equal(t, 2, s.peek())

		s.push(4)
		s.push(5)
		require.Equal(t, 3, s.len())
		require.Equal(t, 5, s.peek())
	})

	t.Run("zero capacity stack", func(t *testing.T) {
		s := newStack[int](0)
		require.Equal(t, 0, s.len())
		require.Equal(t, 0, s.capacity())

		// Should still be able to push (will grow)
		s.push(42)
		require.Equal(t, 1, s.len())
		require.Equal(t, 42, s.peek())
	})
}
