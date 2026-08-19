package resolve

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"
)

func TestResponseExtensionAccumulatorSnapshotIsImmutable(t *testing.T) {
	first := astjson.ObjectValue(nil).GetObject()
	second := astjson.ObjectValue(nil).GetObject()
	accumulator := &responseExtensionAccumulator{}

	accumulator.append(first)
	snapshot := accumulator.snapshot()
	accumulator.append(second)

	require.Equal(t, []*astjson.Object{first}, snapshot)
	require.Equal(t, []*astjson.Object{first, second}, accumulator.snapshot())
}

func TestResponseExtensionAccumulatorValueCompletionSuppressionIsSticky(t *testing.T) {
	accumulator := &responseExtensionAccumulator{}
	require.False(t, accumulator.valueCompletionSuppressed())
	accumulator.suppressValueCompletion()
	require.True(t, accumulator.valueCompletionSuppressed())
}

func TestResponseExtensionAccumulatorNilReceiverIsSafe(t *testing.T) {
	var accumulator *responseExtensionAccumulator
	accumulator.append(nil)
	accumulator.suppressValueCompletion()
	require.Nil(t, accumulator.snapshot())
	require.False(t, accumulator.valueCompletionSuppressed())
}
