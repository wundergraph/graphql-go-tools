//go:build integration

package searche2e

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex/pgvector"
)

func startPgvectorContainer(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "pgvector/pgvector:pg16",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "test",
			"POSTGRES_PASSWORD": "test",
			"POSTGRES_DB":       "testdb",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start pgvector container: %v", err)
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
	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatalf("failed to get mapped port: %v", err)
	}

	dsn := fmt.Sprintf("postgres://test:test@%s:%s/testdb?sslmode=disable", host, port.Port())

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Logf("failed to close database: %v", err)
		}
	})

	// Wait for database to be ready with a ping loop.
	for i := 0; i < 30; i++ {
		if err := db.PingContext(ctx); err == nil {
			return db
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("database did not become ready after 30 ping attempts")
	return nil
}

func newPgvectorIndex(t *testing.T, db *sql.DB) searchindex.Index {
	t.Helper()

	factory := pgvector.NewFactory(db)
	schema := ProductIndexSchema()

	indexName := fmt.Sprintf("test_%d", time.Now().UnixNano())
	idx, err := factory.CreateIndex(context.Background(), indexName, schema, nil)
	if err != nil {
		t.Fatalf("CreateIndex: %v", err)
	}
	t.Cleanup(func() { _ = idx.Close() })

	return idx
}

func TestPgvector(t *testing.T) {
	db := startPgvectorContainer(t)

	idx := newPgvectorIndex(t, db)

	RunBackendTests(t, idx, BackendCaps{
		HasTextSearch:   true,
		HasFacets:       true,
		HasPrefix:       true,
		HasExists:       true,
		HasVectorSearch: true,
	}, BackendHooks{}, func(t *testing.T) searchindex.Index {
		return newPgvectorIndex(t, db)
	})
}
