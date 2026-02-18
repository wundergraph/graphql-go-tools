//go:build integration

package searchtesting

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

const pgvectorConfigSDL = `
extend schema @index(name: "products", backend: "pgvector", config: "{}")

type Product @key(fields: "id") @searchable(index: "products", searchField: "searchProducts") {
  id: ID!
  name: String @indexed(type: TEXT, filterable: true, sortable: true)
  description: String @indexed(type: TEXT)
  category: String @indexed(type: KEYWORD, filterable: true, sortable: true)
  price: Float @indexed(type: NUMERIC, filterable: true, sortable: true)
  inStock: Boolean @indexed(type: BOOL, filterable: true)
}
`

func startPgvector(t *testing.T) *sql.DB {
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
	t.Cleanup(func() { db.Close() })

	// Wait for database to be ready.
	for i := 0; i < 30; i++ {
		if err := db.PingContext(ctx); err == nil {
			return db
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("database did not become ready after 30 ping attempts")
	return nil
}

func TestPgvector(t *testing.T) {
	t.Parallel()
	db := startPgvector(t)

	makeSetup := func(name, configSDL string) BackendSetup {
		return BackendSetup{
			Name:      name,
			ConfigSDL: configSDL,
			CreateIndex: func(t *testing.T, name string, schema searchindex.IndexConfig, _ []byte) searchindex.Index {
				t.Helper()
				factory := pgvector.NewFactory(db)
				idx, err := factory.CreateIndex(context.Background(), name, schema, nil)
				if err != nil {
					t.Fatalf("CreateIndex: %v", err)
				}
				t.Cleanup(func() { idx.Close() })
				return idx
			},
			Caps: BackendCaps{
				HasTextSearch: true,
				HasFacets:     true,
			},
		}
	}

	// Pgvector subtests run sequentially because CREATE EXTENSION races on the same DB.
	t.Run("standard", func(t *testing.T) {
		setup := makeSetup("pgvector", pgvectorConfigSDL)
		setup.ExpectedResponses = map[string]string{
			"supergraph_sdl":                  expectedSupergraphSDL,
			"basic_search_with_entity_join":   `{"data":{"searchProducts":{"hits":[{"node":{"id":"1","name":"Running Shoes","price":89.99,"manufacturer":"Nike"}},{"node":{"id":"2","name":"Basketball Shoes","price":129.99,"manufacturer":"Adidas"}}],"totalCount":2}}}`,
			"filter_keyword_with_entity_join": `{"data":{"searchProducts":{"hits":[{"node":{"id":"4","name":"Wool Socks","rating":4.7}},{"node":{"id":"1","name":"Running Shoes","rating":4.5}},{"node":{"id":"2","name":"Basketball Shoes","rating":4.2}}]}}}`,
			"filter_boolean":                  `{"data":{"searchProducts":{"hits":[{"node":{"id":"3","manufacturer":"Gucci"}}],"totalCount":1}}}`,
			"filter_numeric_range":            `{"data":{"searchProducts":{"hits":[{"node":{"id":"3","manufacturer":"Gucci"}},{"node":{"id":"1","manufacturer":"Nike"}}],"totalCount":2}}}`,
			"filter_AND":                      `{"data":{"searchProducts":{"hits":[{"node":{"id":"4","manufacturer":"Smartwool"}},{"node":{"id":"1","manufacturer":"Nike"}},{"node":{"id":"2","manufacturer":"Adidas"}}],"totalCount":3}}}`,
			"filter_OR":                       `{"data":{"searchProducts":{"hits":[{"node":{"id":"3","manufacturer":"Gucci"}},{"node":{"id":"2","manufacturer":"Adidas"}}],"totalCount":2}}}`,
			"filter_NOT":                      `{"data":{"searchProducts":{"hits":[{"node":{"id":"3","manufacturer":"Gucci"}}],"totalCount":1}}}`,
			"sort_with_entity_join":           `{"data":{"searchProducts":{"hits":[{"node":{"id":"4","name":"Wool Socks","price":12.99,"manufacturer":"Smartwool"}},{"node":{"id":"3","name":"Leather Belt","price":35,"manufacturer":"Gucci"}},{"node":{"id":"1","name":"Running Shoes","price":89.99,"manufacturer":"Nike"}},{"node":{"id":"2","name":"Basketball Shoes","price":129.99,"manufacturer":"Adidas"}}]}}}`,
			"pagination_with_entity_join":     `{"data":{"searchProducts":{"hits":[{"node":{"id":"3","reviews":[{"text":"Nice belt","stars":3}]}},{"node":{"id":"1","reviews":[{"text":"Great shoes","stars":5}]}}],"totalCount":4}}}`,
			"score_and_totalCount":            `{"data":{"searchProducts":{"hits":[{"score":0,"node":{"id":"4","manufacturer":"Smartwool"}},{"score":0,"node":{"id":"3","manufacturer":"Gucci"}},{"score":0,"node":{"id":"1","manufacturer":"Nike"}},{"score":0,"node":{"id":"2","manufacturer":"Adidas"}}],"totalCount":4}}}`,
			"facets_with_entity_join":         `{"data":{"searchProducts":{"hits":[{"node":{"id":"4","manufacturer":"Smartwool"}},{"node":{"id":"3","manufacturer":"Gucci"}},{"node":{"id":"1","manufacturer":"Nike"}},{"node":{"id":"2","manufacturer":"Adidas"}}],"facets":[{"field":"category","values":[{"value":"Footwear","count":3},{"value":"Accessories","count":1}]}]}}}`,
		}
		RunAllScenarios(t, setup)
	})

	t.Run("vector", func(t *testing.T) {
		RunVectorScenarios(t, VectorBackendSetup{
			BackendSetup: makeSetup("pgvector_vector", vectorConfigSDL("pgvector", "{}")),
			Embedder:     &MockEmbedder{},
		})
	})

	t.Run("suggest", func(t *testing.T) {
		RunSuggestScenarios(t, makeSetup("pgvector_suggest", suggestConfigSDL("pgvector", "{}")))
	})

	t.Run("date", func(t *testing.T) {
		setup := makeSetup("pgvector_date", dateConfigSDL("pgvector", "{}"))
		setup.ExpectedResponses = map[string]string{
			"date_eq_filter":         `{"data":{"searchProducts":{"hits":[{"node":{"id":"1","name":"Running Shoes","manufacturer":"Nike"}}],"totalCount":1}}}`,
			"date_range_gte_lte":     `{"data":{"searchProducts":{"hits":[{"node":{"id":"1","name":"Running Shoes","manufacturer":"Nike"}},{"node":{"id":"2","name":"Basketball Shoes","manufacturer":"Adidas"}},{"node":{"id":"3","name":"Leather Belt","manufacturer":"Gucci"}}],"totalCount":3}}}`,
			"date_gt_lt":            `{"data":{"searchProducts":{"hits":[{"node":{"id":"2","name":"Basketball Shoes","manufacturer":"Adidas"}},{"node":{"id":"3","name":"Leather Belt","manufacturer":"Gucci"}}],"totalCount":2}}}`,
			"date_after_before":      `{"data":{"searchProducts":{"hits":[{"node":{"id":"3","name":"Leather Belt","manufacturer":"Gucci"}}],"totalCount":1}}}`,
			"datetime_eq_filter":     `{"data":{"searchProducts":{"hits":[{"node":{"id":"2","name":"Basketball Shoes","manufacturer":"Adidas"}}],"totalCount":1}}}`,
			"datetime_range_gte_lte": `{"data":{"searchProducts":{"hits":[{"node":{"id":"1","name":"Running Shoes","manufacturer":"Nike"}},{"node":{"id":"2","name":"Basketball Shoes","manufacturer":"Adidas"}},{"node":{"id":"3","name":"Leather Belt","manufacturer":"Gucci"}}],"totalCount":3}}}`,
			"datetime_after_before":  `{"data":{"searchProducts":{"hits":[{"node":{"id":"4","name":"Wool Socks","manufacturer":"Smartwool"}}],"totalCount":1}}}`,
			"date_sort_asc":          `{"data":{"searchProducts":{"hits":[{"node":{"id":"1","name":"Running Shoes","manufacturer":"Nike"}},{"node":{"id":"2","name":"Basketball Shoes","manufacturer":"Adidas"}},{"node":{"id":"3","name":"Leather Belt","manufacturer":"Gucci"}},{"node":{"id":"4","name":"Wool Socks","manufacturer":"Smartwool"}}]}}}`,
			"date_sort_desc":         `{"data":{"searchProducts":{"hits":[{"node":{"id":"4","name":"Wool Socks","manufacturer":"Smartwool"}},{"node":{"id":"3","name":"Leather Belt","manufacturer":"Gucci"}},{"node":{"id":"2","name":"Basketball Shoes","manufacturer":"Adidas"}},{"node":{"id":"1","name":"Running Shoes","manufacturer":"Nike"}}]}}}`,
			"datetime_sort_asc":      `{"data":{"searchProducts":{"hits":[{"node":{"id":"1","name":"Running Shoes","manufacturer":"Nike"}},{"node":{"id":"2","name":"Basketball Shoes","manufacturer":"Adidas"}},{"node":{"id":"3","name":"Leather Belt","manufacturer":"Gucci"}},{"node":{"id":"4","name":"Wool Socks","manufacturer":"Smartwool"}}]}}}`,
			"date_combined_filter":   `{"data":{"searchProducts":{"hits":[{"node":{"id":"2","name":"Basketball Shoes","manufacturer":"Adidas"}},{"node":{"id":"4","name":"Wool Socks","manufacturer":"Smartwool"}}],"totalCount":2}}}`,
		}
		RunDateScenarios(t, setup)
	})
}
