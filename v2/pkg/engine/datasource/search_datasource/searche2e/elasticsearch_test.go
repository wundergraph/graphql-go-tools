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
	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex/elasticsearch"
)

func startElasticsearch(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "docker.elastic.co/elasticsearch/elasticsearch:8.13.4",
		ExposedPorts: []string{"9200/tcp"},
		Env: map[string]string{
			"discovery.type":         "single-node",
			"xpack.security.enabled": "false",
			"ES_JAVA_OPTS":           "-Xms512m -Xmx512m",
		},
		WaitingFor: wait.ForHTTP("/").
			WithPort("9200/tcp").
			WithStartupTimeout(120 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start elasticsearch container: %v", err)
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
	port, err := container.MappedPort(ctx, "9200/tcp")
	if err != nil {
		t.Fatalf("failed to get mapped port: %v", err)
	}

	return fmt.Sprintf("http://%s:%s", host, port.Port())
}

func newElasticsearchIndex(t *testing.T, baseURL string) searchindex.Index {
	t.Helper()

	factory := elasticsearch.NewFactory()
	cfg := elasticsearch.Config{
		Addresses: []string{baseURL},
	}
	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	indexName := fmt.Sprintf("test-products-%d", time.Now().UnixNano())
	idx, err := factory.CreateIndex(context.Background(), indexName, ProductIndexSchema(), cfgJSON)
	if err != nil {
		t.Fatalf("CreateIndex: %v", err)
	}
	t.Cleanup(func() { idx.Close() })
	return idx
}

func TestElasticsearch(t *testing.T) {
	baseURL := startElasticsearch(t)

	idx := newElasticsearchIndex(t, baseURL)
	RunBackendTests(t, idx, BackendCaps{
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
