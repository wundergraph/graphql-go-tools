package searchindex

import "context"

// Index is the core interface for a search index backend.
type Index interface {
	// IndexDocument indexes a single document.
	IndexDocument(ctx context.Context, doc EntityDocument) error
	// IndexDocuments indexes a batch of documents.
	IndexDocuments(ctx context.Context, docs []EntityDocument) error
	// DeleteDocument deletes a single document by identity.
	DeleteDocument(ctx context.Context, id DocumentIdentity) error
	// DeleteDocuments deletes a batch of documents by identity.
	DeleteDocuments(ctx context.Context, ids []DocumentIdentity) error
	// Search performs a search query and returns results.
	Search(ctx context.Context, req SearchRequest) (*SearchResult, error)
	// Autocomplete returns terms from the index dictionary matching the given prefix.
	Autocomplete(ctx context.Context, req AutocompleteRequest) (*AutocompleteResult, error)
	// Close releases resources held by the index.
	Close() error
}

// AutocompleteRequest describes a term-prefix autocomplete query.
type AutocompleteRequest struct {
	Field  string
	Prefix string
	Limit  int
}

// AutocompleteResult contains matching terms from the index dictionary.
type AutocompleteResult struct {
	Terms []AutocompleteTerm
}

// AutocompleteTerm is a single term with its document count.
type AutocompleteTerm struct {
	Term  string
	Count int
}

// IndexFactory creates Index instances for a specific backend.
type IndexFactory interface {
	// CreateIndex creates a new index with the given name and configuration.
	CreateIndex(ctx context.Context, name string, schema IndexConfig, configJSON []byte) (Index, error)
}
