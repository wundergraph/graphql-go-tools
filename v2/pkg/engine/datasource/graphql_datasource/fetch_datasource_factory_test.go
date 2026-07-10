package graphql_datasource

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFetchModeValues(t *testing.T) {
	require.Equal(t, FetchMode(0), FetchModeSingle)
	require.Equal(t, FetchMode(1), FetchModeEntity)
	require.Equal(t, FetchMode(2), FetchModeEntityBatch)
}
