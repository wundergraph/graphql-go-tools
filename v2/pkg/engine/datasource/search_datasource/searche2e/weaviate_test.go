//go:build integration

package searche2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex/weaviate"
)

func startWeaviate(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "semitechnologies/weaviate:1.27.0",
			ExposedPorts: []string{"8080/tcp"},
			Env: map[string]string{
				"AUTHENTICATION_ANONYMOUS_ACCESS_ENABLED": "true",
				"PERSISTENCE_DATA_PATH":                   "/var/lib/weaviate",
				"DEFAULT_VECTORIZER_MODULE":                "none",
				"CLUSTER_HOSTNAME":                         "node1",
			},
			WaitingFor: wait.ForHTTP("/v1/.well-known/ready").
				WithPort("8080/tcp").
				WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("failed to start weaviate container: %v", err)
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
	port, err := container.MappedPort(ctx, "8080/tcp")
	if err != nil {
		t.Fatalf("failed to get mapped port: %v", err)
	}

	return fmt.Sprintf("%s:%s", host, port.Port())
}

func newWeaviateIndex(t *testing.T, host string, name string) searchindex.Index {
	t.Helper()

	factory := weaviate.NewFactory()
	configJSON := []byte(fmt.Sprintf(`{"host":%q,"scheme":"http"}`, host))

	idx, err := factory.CreateIndex(context.Background(), name, ProductIndexSchema(), configJSON)
	if err != nil {
		t.Fatalf("CreateIndex: %v", err)
	}
	t.Cleanup(func() { idx.Close() })
	return idx
}

func TestWeaviate(t *testing.T) {
	host := startWeaviate(t)

	idx := newWeaviateIndex(t, host, "test_products")
	RunBackendTests(t, idx, BackendCaps{
		HasTextSearch: true,
		HasFacets:     false,
		HasPrefix:     true,
		HasExists:     false,
	}, BackendHooks{
		WaitForIndex: func(t *testing.T) {
			time.Sleep(1 * time.Second)
		},
	}, func(t *testing.T) searchindex.Index {
		name := fmt.Sprintf("weaviate_fresh_%d", time.Now().UnixNano())
		return newWeaviateIndex(t, host, name)
	})
}
