package engine_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func compactJSONForAssert(t testing.TB, input string) string {
	t.Helper()

	var value any
	err := json.Unmarshal([]byte(input), &value)
	require.NoError(t, err)

	normalized, err := json.Marshal(value)
	require.NoError(t, err)
	return string(normalized)
}
