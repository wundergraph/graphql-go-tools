//go:build integration

package searche2e

import (
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

func TestFederation_Meilisearch(t *testing.T) {
	meiliHost := startMeilisearchContainer(t)

	idx := newMeilisearchIndex(t, meiliHost)
	RunFederatedBackendTests(t, idx, BackendCaps{
		HasTextSearch: true,
		HasFacets:     true,
		HasPrefix:     false,
		HasExists:     false,
	}, BackendHooks{}, func(t *testing.T) searchindex.Index {
		return newMeilisearchIndex(t, meiliHost)
	})
}
