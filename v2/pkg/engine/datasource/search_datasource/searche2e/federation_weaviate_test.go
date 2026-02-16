//go:build integration

package searche2e

import (
	"testing"
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

func TestFederation_Weaviate(t *testing.T) {
	host := startWeaviate(t)

	idx := newWeaviateIndex(t, host, "FederationProducts")
	RunFederatedBackendTests(t, idx, BackendCaps{
		HasTextSearch: true,
		HasFacets:     false,
		HasPrefix:     true,
		HasExists:     false,
	}, BackendHooks{
		WaitForIndex: func(t *testing.T) {
			time.Sleep(1 * time.Second)
		},
	}, func(t *testing.T) searchindex.Index {
		return newWeaviateIndex(t, host, "FederationProducts")
	})
}
