package searche2e

import (
	"context"
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex/bleve"
)

func newBleveIndex(t *testing.T) searchindex.Index {
	t.Helper()
	factory := bleve.NewFactory()
	idx, err := factory.CreateIndex(context.Background(), "test", ProductIndexSchema(), nil)
	if err != nil {
		t.Fatalf("CreateIndex: %v", err)
	}
	t.Cleanup(func() { idx.Close() })
	return idx
}

func TestBleve(t *testing.T) {
	idx := newBleveIndex(t)
	RunBackendTests(t, idx, BackendCaps{
		HasTextSearch: true,
		HasFacets:     true,
		HasPrefix:     true,
		HasExists:     true,
	}, BackendHooks{}, func(t *testing.T) searchindex.Index {
		return newBleveIndex(t)
	})
}

func TestBleveCursor(t *testing.T) {
	idx := newBleveIndex(t)
	RunCursorTests(t, idx, BackendCaps{
		HasTextSearch:       true,
		HasCursorPagination: true,
	}, BackendHooks{})
}

func TestEntityJoinCompatibility(t *testing.T) {
	idx := newBleveIndex(t)
	if err := idx.IndexDocuments(context.Background(), TestProducts()); err != nil {
		t.Fatalf("populate: %v", err)
	}

	config := ProductDatasourceConfig()
	source := CreateSource(t, idx, config)

	// Verify Source.Load response has the correct format for federation entity resolution.
	resp := LoadAndParse(t, source, BuildSearchInput(WithLimit(10)))
	if len(resp.Hits) == 0 {
		t.Fatal("expected hits")
	}

	for i, hit := range resp.Hits {
		// Each hit.node must contain __typename + key fields.
		typename, ok := hit.Node["__typename"].(string)
		if !ok || typename != "Product" {
			t.Errorf("hit[%d]: __typename = %v, want Product", i, hit.Node["__typename"])
		}

		id, ok := hit.Node["id"]
		if !ok {
			t.Errorf("hit[%d]: missing key field 'id'", i)
		}
		if _, ok := id.(string); !ok {
			t.Errorf("hit[%d]: id should be string, got %T", i, id)
		}

		// Score or distance should be set for scored queries; for match-all, 0 is acceptable.
	}
}
