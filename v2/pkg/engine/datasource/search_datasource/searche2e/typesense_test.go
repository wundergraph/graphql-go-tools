//go:build integration

package searche2e

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex/typesense"
)

func startTypesense(t *testing.T) (string, int) {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "typesense/typesense:27.1",
		ExposedPorts: []string{"8108/tcp"},
		Env: map[string]string{
			"TYPESENSE_API_KEY":  "test-api-key",
			"TYPESENSE_DATA_DIR": "/data",
		},
		Tmpfs: map[string]string{"/data": ""},
		WaitingFor: wait.ForHTTP("/health").
			WithPort("8108/tcp").
			WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start typesense container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "8108/tcp")
	if err != nil {
		t.Fatalf("failed to get mapped port: %v", err)
	}

	return host, port.Int()
}

func newTypesenseIndex(t *testing.T, host string, port int) searchindex.Index {
	t.Helper()

	factory := typesense.NewFactory()
	cfg := typesense.Config{
		Host:     host,
		Port:     port,
		APIKey:   "test-api-key",
		Protocol: "http",
	}
	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	indexName := fmt.Sprintf("test_%d", time.Now().UnixNano())
	idx, err := factory.CreateIndex(context.Background(), indexName, ProductIndexSchema(), cfgJSON)
	if err != nil {
		t.Fatalf("CreateIndex: %v", err)
	}
	t.Cleanup(func() { idx.Close() })
	return idx
}

func TestTypesense(t *testing.T) {
	host, port := startTypesense(t)

	idx := newTypesenseIndex(t, host, port)
	RunBackendTests(t, idx, BackendCaps{
		HasTextSearch: true,
		HasFacets:     true,
		HasPrefix:     false,
		HasExists:     false,
	}, BackendHooks{}, func(t *testing.T) searchindex.Index {
		return newTypesenseIndex(t, host, port)
	})
}
