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
	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex/qdrant"
)

func startQdrant(t *testing.T) (host string, port int) {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "qdrant/qdrant:v1.12.5",
		ExposedPorts: []string{"6333/tcp"},
		WaitingFor:   wait.ForHTTP("/healthz").WithPort("6333/tcp").WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start qdrant container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate qdrant container: %v", err)
		}
	})

	mappedHost, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get container host: %v", err)
	}
	mappedPort, err := container.MappedPort(ctx, "6333")
	if err != nil {
		t.Fatalf("failed to get mapped port: %v", err)
	}

	return mappedHost, mappedPort.Int()
}

func newQdrantIndex(t *testing.T, host string, port int) searchindex.Index {
	t.Helper()
	factory := qdrant.NewFactory()
	name := fmt.Sprintf("test_%d", time.Now().UnixNano())
	cfgJSON, err := json.Marshal(qdrant.Config{Host: host, Port: port})
	if err != nil {
		t.Fatalf("marshal qdrant config: %v", err)
	}
	idx, err := factory.CreateIndex(context.Background(), name, ProductIndexSchema(), cfgJSON)
	if err != nil {
		t.Fatalf("CreateIndex: %v", err)
	}
	t.Cleanup(func() { idx.Close() })
	return idx
}

func TestQdrant(t *testing.T) {
	host, port := startQdrant(t)
	idx := newQdrantIndex(t, host, port)

	RunBackendTests(t, idx, BackendCaps{
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
