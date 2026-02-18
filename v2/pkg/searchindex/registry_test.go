package searchindex

import (
	"context"
	"testing"
)

type mockFactory struct{}

func (f *mockFactory) CreateIndex(_ context.Context, _ string, _ IndexConfig, _ []byte) (Index, error) {
	return nil, nil
}

type mockEmbedder struct {
	dims int
}

func (e *mockEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i := range result {
		result[i] = make([]float32, e.dims)
	}
	return result, nil
}

func (e *mockEmbedder) EmbedSingle(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, e.dims), nil
}

func (e *mockEmbedder) Dimensions() int { return e.dims }

func TestIndexFactoryRegistry(t *testing.T) {
	reg := NewIndexFactoryRegistry()

	// Get non-existent
	_, err := reg.Get("bleve")
	if err == nil {
		t.Fatal("expected error for non-existent backend")
	}

	// Register and get
	reg.Register("bleve", &mockFactory{})
	f, err := reg.Get("bleve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f == nil {
		t.Fatal("expected non-nil factory")
	}
}

func TestEmbedderRegistry(t *testing.T) {
	reg := NewEmbedderRegistry()

	// Get non-existent
	_, err := reg.Get("text-embedding-3-small")
	if err == nil {
		t.Fatal("expected error for non-existent model")
	}

	// Register and get
	reg.Register("text-embedding-3-small", &mockEmbedder{dims: 1536})
	e, err := reg.Get("text-embedding-3-small")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.Dimensions() != 1536 {
		t.Errorf("Dimensions() = %d, want 1536", e.Dimensions())
	}
}
