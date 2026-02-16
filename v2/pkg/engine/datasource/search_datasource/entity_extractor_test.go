package search_datasource

import (
	"testing"
)

func TestExtractEntities(t *testing.T) {
	t.Run("extract array from path", func(t *testing.T) {
		response := []byte(`{
			"data": {
				"products": [
					{"id": "1", "name": "Widget", "price": 9.99},
					{"id": "2", "name": "Gadget", "price": 19.99}
				]
			}
		}`)

		docs, err := ExtractEntities(response, "data.products", "Product", []string{"id"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(docs) != 2 {
			t.Fatalf("expected 2 documents, got %d", len(docs))
		}

		if docs[0].Identity.TypeName != "Product" {
			t.Errorf("TypeName = %q, want %q", docs[0].Identity.TypeName, "Product")
		}
		if docs[0].Identity.KeyFields["id"] != "1" {
			t.Errorf("KeyFields[id] = %v, want %q", docs[0].Identity.KeyFields["id"], "1")
		}
		if docs[0].Fields["name"] != "Widget" {
			t.Errorf("Fields[name] = %v, want %q", docs[0].Fields["name"], "Widget")
		}
	})

	t.Run("extract single object", func(t *testing.T) {
		response := []byte(`{
			"data": {
				"productUpdated": {"id": "1", "name": "Widget", "price": 9.99}
			}
		}`)

		docs, err := ExtractEntities(response, "data.productUpdated", "Product", []string{"id"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(docs) != 1 {
			t.Fatalf("expected 1 document, got %d", len(docs))
		}
	})

	t.Run("missing key field", func(t *testing.T) {
		response := []byte(`{"data": {"items": [{"name": "Widget"}]}}`)

		_, err := ExtractEntities(response, "data.items", "Product", []string{"id"})
		if err == nil {
			t.Fatal("expected error for missing key field")
		}
	})

	t.Run("invalid path", func(t *testing.T) {
		response := []byte(`{"data": {}}`)

		_, err := ExtractEntities(response, "data.missing", "Product", []string{"id"})
		if err == nil {
			t.Fatal("expected error for invalid path")
		}
	})

	t.Run("vector fields extracted separately", func(t *testing.T) {
		response := []byte(`{
			"data": {
				"images": [
					{"id": "1", "caption": "A cat", "embedding": [0.1, 0.2, 0.3]}
				]
			}
		}`)

		docs, err := ExtractEntities(response, "data.images", "Image", []string{"id"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(docs) != 1 {
			t.Fatalf("expected 1 document, got %d", len(docs))
		}

		// Embedding should be in Vectors, not Fields
		if _, ok := docs[0].Fields["embedding"]; ok {
			t.Error("embedding should not be in Fields")
		}
		vec, ok := docs[0].Vectors["embedding"]
		if !ok {
			t.Fatal("embedding should be in Vectors")
		}
		if len(vec) != 3 {
			t.Errorf("vector length = %d, want 3", len(vec))
		}
	})
}

func TestEntityFieldMaps(t *testing.T) {
	response := []byte(`{
		"data": {
			"articles": [
				{"id": "1", "title": "Hello", "body": "World"},
				{"id": "2", "title": "Foo", "body": "Bar"}
			]
		}
	}`)

	docs, err := ExtractEntities(response, "data.articles", "Article", []string{"id"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fieldMaps := EntityFieldMaps(docs)
	if len(fieldMaps) != 2 {
		t.Fatalf("expected 2 field maps, got %d", len(fieldMaps))
	}
	if fieldMaps[0]["title"] != "Hello" {
		t.Errorf("fieldMaps[0][title] = %v, want %q", fieldMaps[0]["title"], "Hello")
	}
}
