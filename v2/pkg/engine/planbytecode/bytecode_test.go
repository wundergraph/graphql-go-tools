package planbytecode

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

func TestOpIsFixedWidth(t *testing.T) {
	require.Equal(t, uintptr(16), unsafe.Sizeof(Op{}))
}
