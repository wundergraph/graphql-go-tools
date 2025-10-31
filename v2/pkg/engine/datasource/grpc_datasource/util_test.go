package grpcdatasource

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAncestor(t *testing.T) {
	ancestor := newStack[int](10)
	ancestor.push(1)
	ancestor.push(2)
	ancestor.push(3)
	require.Equal(t, 10, ancestor.capacity())
	require.Equal(t, 3, ancestor.len())
	ancestor.pop()
	require.Equal(t, 2, ancestor.len())
	require.Equal(t, 2, ancestor.peek())
	require.Equal(t, 2, ancestor.len())
}
