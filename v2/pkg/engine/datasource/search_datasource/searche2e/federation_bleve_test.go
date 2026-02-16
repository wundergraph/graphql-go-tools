package searche2e

import (
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

func TestFederation_Bleve(t *testing.T) {
	t.Parallel()
	idx := newBleveIndex(t)
	RunFederatedBackendTests(t, idx, BackendCaps{
		HasTextSearch: true,
		HasFacets:     true,
		HasPrefix:     true,
		HasExists:     true,
	}, BackendHooks{}, func(t *testing.T) searchindex.Index {
		return newBleveIndex(t)
	})
}
