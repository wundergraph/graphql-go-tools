package search_datasource

import "github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"

// Configuration is the DataSource-specific configuration for the search datasource.
type Configuration struct {
	// IndexName is the name of the search index.
	IndexName string `json:"index_name"`
	// SearchField is the name of the generated Query field.
	SearchField string `json:"search_field"`
	// EntityTypeName is the entity type this search field resolves.
	EntityTypeName string `json:"entity_type_name"`
	// KeyFields are the federation key fields for the entity.
	KeyFields []string `json:"key_fields"`
	// Fields describes the indexed fields and their types.
	Fields []IndexedFieldConfig `json:"fields"`
	// EmbeddingFields describes derived embedding fields.
	EmbeddingFields []EmbeddingFieldConfig `json:"embedding_fields,omitempty"`
	// HasVectorSearch indicates the entity supports vector search.
	HasVectorSearch bool `json:"has_vector_search"`
	// HasTextSearch indicates the entity supports full-text search.
	HasTextSearch bool `json:"has_text_search"`
	// ResultsMetaInformation controls whether the search field returns wrapper types with score/distance
	// or a flat entity array. Defaults to true.
	ResultsMetaInformation bool `json:"results_meta_information"`
	// CursorBasedPagination enables Relay-style cursor pagination.
	CursorBasedPagination bool `json:"cursor_based_pagination,omitempty"`
	// CursorBidirectional enables last/before args (true for bleve, pgvector; false for elasticsearch).
	CursorBidirectional bool `json:"cursor_bidirectional,omitempty"`
	// IsSuggest indicates this configuration is for the suggest/autocomplete field, not the search field.
	IsSuggest bool `json:"is_suggest,omitempty"`
}

// NeedsResponseWrapper returns true if the config requires wrapper types.
func (c *Configuration) NeedsResponseWrapper() bool {
	return c.ResultsMetaInformation || c.CursorBasedPagination
}

// IndexedFieldConfig describes a field's indexing configuration.
type IndexedFieldConfig struct {
	FieldName    string               `json:"field_name"`
	GraphQLType  string               `json:"graphql_type"`
	IndexType    searchindex.FieldType `json:"index_type"`
	Filterable   bool                 `json:"filterable"`
	Sortable     bool                 `json:"sortable"`
	Dimensions   int                  `json:"dimensions,omitempty"`
	Weight       float64              `json:"weight,omitempty"`
	Autocomplete bool                 `json:"autocomplete,omitempty"`
}

// EmbeddingFieldConfig describes a derived embedding field.
type EmbeddingFieldConfig struct {
	FieldName    string   `json:"field_name"`
	SourceFields []string `json:"source_fields"`
	Template     string   `json:"template"`
	Model        string   `json:"model"`
}
