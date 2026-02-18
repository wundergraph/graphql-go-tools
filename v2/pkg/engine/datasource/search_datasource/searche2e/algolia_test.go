//go:build integration

package searche2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex/algolia"
)

func skipIfNoAlgolia(t *testing.T) (string, string) {
	t.Helper()
	appID := os.Getenv("ALGOLIA_APP_ID")
	apiKey := os.Getenv("ALGOLIA_API_KEY")
	if appID == "" || apiKey == "" {
		t.Skip("ALGOLIA_APP_ID and ALGOLIA_API_KEY environment variables are required for integration tests")
	}
	return appID, apiKey
}

func newAlgoliaIndex(t *testing.T, appID, apiKey string) searchindex.Index {
	t.Helper()

	factory := &algolia.Factory{}
	cfg := algolia.Config{
		AppID:  appID,
		APIKey: apiKey,
	}
	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	indexName := fmt.Sprintf("test_e2e_%d", time.Now().UnixNano())
	idx, err := factory.CreateIndex(context.Background(), indexName, ProductIndexSchema(), cfgJSON)
	if err != nil {
		t.Fatalf("CreateIndex: %v", err)
	}
	t.Cleanup(func() { idx.Close() })
	return idx
}

func TestAlgolia(t *testing.T) {
	appID, apiKey := skipIfNoAlgolia(t)

	idx := newAlgoliaIndex(t, appID, apiKey)
	RunBackendTests(t, idx, BackendCaps{
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
