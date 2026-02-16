//go:build integration

package searche2e

import (
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

func TestFederation_Pgvector(t *testing.T) {
	db := startPgvectorContainer(t)

	idx := newPgvectorIndex(t, db)
	RunFederatedBackendTests(t, idx, BackendCaps{
		HasTextSearch:   true,
		HasFacets:       true,
		HasPrefix:       true,
		HasExists:       true,
		HasVectorSearch: true,
	}, BackendHooks{}, func(t *testing.T) searchindex.Index {
		return newPgvectorIndex(t, db)
	})
}
