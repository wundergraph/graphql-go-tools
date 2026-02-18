//go:build integration

package searche2e

import (
	"testing"
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

func TestFederation_Elasticsearch(t *testing.T) {
	baseURL := startElasticsearch(t)

	idx := newElasticsearchIndex(t, baseURL)
	RunFederatedBackendTests(t, idx, BackendCaps{
		HasTextSearch: true,
		HasFacets:     true,
		HasPrefix:     true,
		HasExists:     true,
	}, BackendHooks{
		WaitForIndex: func(t *testing.T) {
			time.Sleep(2 * time.Second)
		},
	}, func(t *testing.T) searchindex.Index {
		return newElasticsearchIndex(t, baseURL)
	})
}
