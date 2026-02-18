package searchtesting

import (
	"context"
	"fmt"

	"github.com/wundergraph/graphql-go-tools/execution/searchtesting/shareddata"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

func testProducts() []searchindex.EntityDocument {
	var docs []searchindex.EntityDocument
	for _, p := range shareddata.Products() {
		docs = append(docs, searchindex.EntityDocument{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": p.ID}},
			Fields:   map[string]any{"name": p.Name, "description": p.Description, "category": p.Category, "price": p.Price, "inStock": p.InStock},
		})
	}
	return docs
}

// testGeoProducts returns the 4 standard test products with geo locations.
func testGeoProducts() []searchindex.EntityDocument {
	var docs []searchindex.EntityDocument
	for _, p := range shareddata.Products() {
		fields := map[string]any{"name": p.Name, "description": p.Description, "category": p.Category, "price": p.Price, "inStock": p.InStock}
		if p.Location != nil {
			fields["location"] = map[string]any{"lat": p.Location.Lat, "lon": p.Location.Lon}
		}
		docs = append(docs, searchindex.EntityDocument{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": p.ID}},
			Fields:   fields,
		})
	}
	return docs
}

// testDateProducts returns the 4 standard test products with date/datetime fields.
func testDateProducts() []searchindex.EntityDocument {
	var docs []searchindex.EntityDocument
	for _, p := range shareddata.Products() {
		fields := map[string]any{"name": p.Name, "description": p.Description, "category": p.Category, "price": p.Price, "inStock": p.InStock}
		fields["createdAt"] = p.CreatedAt
		fields["updatedAt"] = p.UpdatedAt
		docs = append(docs, searchindex.EntityDocument{
			Identity: searchindex.DocumentIdentity{TypeName: "Product", KeyFields: map[string]any{"id": p.ID}},
			Fields:   fields,
		})
	}
	return docs
}

// testVectorProducts returns the same 4 test products with pre-computed embedding vectors.
// The template matches the @embedding directive: "{{name}}. {{description}}".
func testVectorProducts(embedder searchindex.Embedder) []searchindex.EntityDocument {
	docs := testProducts()
	for i := range docs {
		name, _ := docs[i].Fields["name"].(string)
		desc, _ := docs[i].Fields["description"].(string)
		text := fmt.Sprintf("%s. %s", name, desc)
		vec, _ := embedder.EmbedSingle(context.Background(), text)
		docs[i].Vectors = map[string][]float32{
			"_embedding": vec,
		}
	}
	return docs
}
