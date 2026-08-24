package resolve

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// The root fetch cache keys a whole upstream request, so what it may be asked to
// key is narrower than what the loader sees: only a root fetch has a request the
// client's variables fully determine, and only a GraphQL subgraph answers with
// the Cache-Control header the store decision needs.
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
		// Introspection, pubsub and gRPC plugins all answer without a
		// Cache-Control header, so probing the cache for them could only ever
		// cost a round trip.
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
