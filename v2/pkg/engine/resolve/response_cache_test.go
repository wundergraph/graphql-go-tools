package resolve

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/caching"
)

// What the root fetch cache may be asked to key is narrower than what the
// loader sees: one case per condition that narrows it.
func TestRootFetchCacheable(t *testing.T) {
	rootItem := func() *FetchItem { return &FetchItem{} }

	queryFetch := func() *SingleFetch {
		return &SingleFetch{
			FetchConfiguration: FetchConfiguration{
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath:   []string{"data"},
					SelectResponseErrorsPath: []string{"errors"},
				},
			},
			DataSourceIdentifier: []byte("graphql_datasource.Source"),
			Info:                 &FetchInfo{OperationType: ast.OperationTypeQuery},
		}
	}

	t.Run("a root query fetch against a subgraph is cacheable", func(t *testing.T) {
		require.True(t, rootFetchCacheable(rootItem(), queryFetch()))
	})

	t.Run("a nested fetch is not", func(t *testing.T) {
		// Its input carries parent data, which is a different thing to key on
		// than a request the variables determine on their own.
		nested := &FetchItem{FetchPath: []FetchItemPathElement{{Path: []string{"topProducts"}}}}
		require.False(t, rootFetchCacheable(nested, queryFetch()))
	})

	t.Run("a mutation is not", func(t *testing.T) {
		fetch := queryFetch()
		fetch.Info.OperationType = ast.OperationTypeMutation
		require.False(t, rootFetchCacheable(rootItem(), fetch))
	})

	t.Run("a subscription is not", func(t *testing.T) {
		fetch := queryFetch()
		fetch.Info.OperationType = ast.OperationTypeSubscription
		require.False(t, rootFetchCacheable(rootItem(), fetch))
	})

	t.Run("a fetch answered by anything but a GraphQL subgraph is not", func(t *testing.T) {
		// Introspection, pubsub and gRPC plugins answer without a Cache-Control
		// header, so a lookup for them could only ever cost a round trip.
		fetch := queryFetch()
		fetch.DataSourceIdentifier = []byte("introspection_datasource.Source")
		require.False(t, rootFetchCacheable(rootItem(), fetch))
	})

	t.Run("a fetch that reads its data from anywhere but data is not", func(t *testing.T) {
		// A hit rebuilds {"data":...} around the stored bytes, which is only the
		// response this fetch's merge would read back.
		fetch := queryFetch()
		fetch.PostProcessing.SelectResponseDataPath = []string{"data", "_entities"}
		require.False(t, rootFetchCacheable(rootItem(), fetch))
	})

	t.Run("a fetch with no info is not", func(t *testing.T) {
		fetch := queryFetch()
		fetch.Info = nil
		require.False(t, rootFetchCacheable(rootItem(), fetch))
	})
}

func TestRemainingTTL(t *testing.T) {
	item := func(ttl time.Duration) caching.Item { return caching.Item{TTL: ttl} }

	testCases := []struct {
		name     string
		found    map[string]caching.Item
		keys     []string
		expected time.Duration
	}{
		{
			name:     "the shortest of several entries",
			found:    map[string]caching.Item{"a": item(30 * time.Second), "b": item(10 * time.Second)},
			keys:     []string{"a", "b"},
			expected: 10 * time.Second,
		},
		{
			name:     "zero is a lifetime, not an absent one",
			found:    map[string]caching.Item{"a": item(30 * time.Second), "b": item(0)},
			keys:     []string{"a", "b"},
			expected: 0,
		},
		{
			name:     "a lone zero survives",
			found:    map[string]caching.Item{"a": item(0)},
			keys:     []string{"a"},
			expected: 0,
		},
		{
			name:     "a negative TTL is dropped in favour of its neighbours",
			found:    map[string]caching.Item{"a": item(-5 * time.Second), "b": item(10 * time.Second)},
			keys:     []string{"a", "b"},
			expected: 10 * time.Second,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, remainingTTL(tc.found, tc.keys))
		})
	}
}
