//go:build integration

package searche2e

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex/meilisearch"
)

const testMasterKey = "test-master-key"

func startMeilisearchContainer(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "getmeili/meilisearch:v1.6",
		ExposedPorts: []string{"7700/tcp"},
		Env: map[string]string{
			"MEILI_MASTER_KEY": testMasterKey,
		},
		WaitingFor: wait.ForHTTP("/health").WithPort("7700/tcp").WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start Meilisearch container")
	t.Cleanup(func() {
		_ = container.Terminate(ctx)
	})

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "7700")
	require.NoError(t, err)

	return fmt.Sprintf("http://%s:%s", host, port.Port())
}

func newMeilisearchIndex(t *testing.T, meiliHost string) searchindex.Index {
	t.Helper()

	factory := meilisearch.NewFactory()
	schema := ProductIndexSchema()

	cfg := meilisearch.Config{
		Host:   meiliHost,
		APIKey: testMasterKey,
	}
	cfgJSON, err := json.Marshal(cfg)
	require.NoError(t, err)

	indexName := fmt.Sprintf("test_%d", time.Now().UnixNano())
	idx, err := factory.CreateIndex(context.Background(), indexName, schema, cfgJSON)
	require.NoError(t, err, "CreateIndex")
	t.Cleanup(func() { _ = idx.Close() })

	return idx
}

func TestMeilisearch(t *testing.T) {
	meiliHost := startMeilisearchContainer(t)

	idx := newMeilisearchIndex(t, meiliHost)

	RunBackendTests(t, idx, BackendCaps{
		HasTextSearch: true,
		HasFacets:     true,
		HasPrefix:     false,
		HasExists:     false,
	}, BackendHooks{}, func(t *testing.T) searchindex.Index {
		return newMeilisearchIndex(t, meiliHost)
	})
}
