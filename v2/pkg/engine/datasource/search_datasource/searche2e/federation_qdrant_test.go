//go:build integration

package searche2e

import (
	"testing"
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

func TestFederation_Qdrant(t *testing.T) {
	host, port := startQdrant(t)

	idx := newQdrantIndex(t, host, port)
	RunFederatedBackendTests(t, idx, BackendCaps{
		HasTextSearch: false,
		HasFacets:     false,
		HasPrefix:     false,
		HasExists:     false,
	}, BackendHooks{
		WaitForIndex: func(t *testing.T) {
			time.Sleep(1 * time.Second)
		},
	}, func(t *testing.T) searchindex.Index {
		return newQdrantIndex(t, host, port)
	})
}
