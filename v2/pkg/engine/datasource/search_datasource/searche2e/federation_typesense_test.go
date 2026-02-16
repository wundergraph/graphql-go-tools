//go:build integration

package searche2e

import (
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

func TestFederation_Typesense(t *testing.T) {
	host, port := startTypesense(t)

	idx := newTypesenseIndex(t, host, port)
	RunFederatedBackendTests(t, idx, BackendCaps{
		HasTextSearch: true,
		HasFacets:     true,
		HasPrefix:     false,
		HasExists:     false,
	}, BackendHooks{}, func(t *testing.T) searchindex.Index {
		return newTypesenseIndex(t, host, port)
	})
}
