//go:build integration

package searche2e

import (
	"testing"
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

func TestFederation_Algolia(t *testing.T) {
	appID, apiKey := skipIfNoAlgolia(t)

	idx := newAlgoliaIndex(t, appID, apiKey)
	RunFederatedBackendTests(t, idx, BackendCaps{
		HasTextSearch: true,
		HasFacets:     true,
		HasPrefix:     false,
		HasExists:     false,
	}, BackendHooks{
		WaitForIndex: func(t *testing.T) {
			time.Sleep(2 * time.Second)
		},
	}, func(t *testing.T) searchindex.Index {
		return newAlgoliaIndex(t, appID, apiKey)
	})
}
