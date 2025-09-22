package resolve

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/astjson"
)

func TestSelectObjectAndIndex(t *testing.T) {
	tests := []struct {
		name           string
		responseJSON   string
		pathElements   []interface{} // Can be strings or numbers
		expectedEntity string        // JSON string of expected entity, or "nil" for nil
		expectedIndex  int
	}{
		{
			name:           "complex federation-like structure",
			responseJSON:   `[{"__typename": "User", "id": "1", "name": "John"}, {"__typename": "User", "id": "2", "name": null}]`,
			pathElements:   []interface{}{1},
			expectedEntity: `{"__typename": "User", "id": "2", "name": null}`,
			expectedIndex:  1,
		},
		{
			name:           "mixed path with number then string",
			responseJSON:   `[{"user": {"name": "John"}}, {"user": {"name": "Jane"}}]`,
			pathElements:   []interface{}{1, "user"},
			expectedEntity: `{"name": "Jane"}`,
			expectedIndex:  1,
		},
		{
			name:           "multiple numbers in path",
			responseJSON:   `[[{"name": "A"}, {"name": "B"}], [{"name": "C"}, {"name": "D"}]]`,
			pathElements:   []interface{}{1, 0},
			expectedEntity: `{"name": "C"}`,
			expectedIndex:  1,
		},
		{
			name:           "path leads to non-existent key",
			responseJSON:   `[{"user": {"name": "John"}}]`,
			pathElements:   []interface{}{0, "user", "nonexistent"},
			expectedEntity: "nil",
			expectedIndex:  -1,
		},
		{
			name:           "negative index is an error",
			responseJSON:   `[{"name": "A"}, {"name": "negative"}]`,
			pathElements:   []interface{}{-2},
			expectedEntity: "nil",
			expectedIndex:  -1,
		},
		{
			name:           "out of bound index is an error",
			responseJSON:   `[{"name": "A"}, {"name": "negative"}]`,
			pathElements:   []interface{}{9},
			expectedEntity: "nil",
			expectedIndex:  -1,
		},
		{
			name:           "empty path is an error",
			responseJSON:   `[{"name": "A"}, {"name": "negative"}]`,
			pathElements:   []interface{}{},
			expectedEntity: "nil",
			expectedIndex:  -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := astjson.ParseBytesWithoutCache([]byte(tt.responseJSON))
			assert.NoError(t, err, "Failed to parse response JSON")

			// Convert path elements to astjson.Value slice
			path := make([]*astjson.Value, len(tt.pathElements))
			for i, elem := range tt.pathElements {
				switch v := elem.(type) {
				case string:
					path[i] = astjson.MustParse(`"` + v + `"`)
				case int:
					path[i] = astjson.MustParse(fmt.Sprintf("%d", v))
				default:
					t.Fatalf("Unsupported path element type: %T", v)
				}
			}

			entity, index := selectObjectAndIndex(response, path)

			assert.Equal(t, tt.expectedIndex, index, "Index mismatch")

			if tt.expectedEntity == "nil" {
				assert.Nil(t, entity, "Expected nil entity")
			} else {
				assert.NotNil(t, entity, "Expected non-nil entity")
				expectedEntity, err := astjson.ParseBytesWithoutCache([]byte(tt.expectedEntity))
				assert.NoError(t, err, "Failed to parse expected entity JSON")

				// Compare JSON representations
				actualJSON := entity.MarshalTo(nil)
				expectedJSON := expectedEntity.MarshalTo(nil)
				assert.JSONEq(t, string(expectedJSON), string(actualJSON), "Entity content mismatch")
			}
		})
	}
}

func TestGetTaintedIndices(t *testing.T) {
	tests := []struct {
		name            string
		fetchReasons    []FetchReason
		responseJSON    string
		errorsJSON      string
		expectedIndices []int
	}{
		{
			name: "single entity with requires dependency failure",
			fetchReasons: []FetchReason{
				{TypeName: "User", FieldName: "email", IsRequires: true, Nullable: true},
			},
			responseJSON: `[
				{"__typename": "User", "id": "1", "email": null},
				{"__typename": "User", "id": "2", "email": "user2@example.com"}
			]`,
			errorsJSON: `[
				{
					"message": "Cannot resolve field email",
					"path": ["_entities", 0, "email"]
				}
			]`,
			expectedIndices: []int{0},
		},
		{
			name: "multiple entities with requires dependency failures",
			fetchReasons: []FetchReason{
				{TypeName: "Product", FieldName: "reviews", IsRequires: true, Nullable: true},
				{TypeName: "Product", FieldName: "rating", IsRequires: true, Nullable: true},
			},
			responseJSON: `[
				{"__typename": "Product", "upc": "1", "reviews": null, "rating": 4.5},
				{"__typename": "Product", "upc": "2", "reviews": [], "rating": null},
				{"__typename": "Product", "upc": "3", "reviews": [], "rating": 3.8}
			]`,
			errorsJSON: `[
				{
					"message": "Cannot resolve field reviews",
					"path": ["_entities", 0, "reviews"]
				},
				{
					"message": "Cannot resolve field rating", 
					"path": ["_entities", 1, "rating"]
				}
			]`,
			expectedIndices: []int{0, 1},
		},
		{
			name: "error in non-required field should not taint entity",
			fetchReasons: []FetchReason{
				{TypeName: "Product", FieldName: "reviews", IsRequires: true, Nullable: true},
				{TypeName: "Product", FieldName: "description", IsKey: true, Nullable: true},
			},
			responseJSON: `[
				{"__typename": "Product", "upc": "1", "reviews": [], "description": null}
			]`,
			errorsJSON: `[
				{
					"message": "Description not available",
					"path": ["_entities", 0, "description"]
				}
			]`,
			expectedIndices: nil,
		},
		{
			name: "error in non-nullable field should not taint entity",
			fetchReasons: []FetchReason{
				{TypeName: "Product", FieldName: "reviews", IsRequires: true, Nullable: true},
				{TypeName: "Product", FieldName: "description", IsRequires: true, Nullable: false},
			},
			responseJSON: `[
				{"__typename": "Product", "upc": "1", "reviews": [], "description": null}
			]`,
			errorsJSON: `[
				{
					"message": "Description not available",
					"path": ["_entities", 0, "description"]
				}
			]`,
			expectedIndices: nil,
		},
		{
			name: "error path without _entities should be ignored",
			fetchReasons: []FetchReason{
				{TypeName: "User", FieldName: "email", IsRequires: true, Nullable: true},
			},
			responseJSON: `{
				"users": [
					{"__typename": "User", "id": "1", "email": null}
				]
			}`,
			errorsJSON: `[
				{
					"message": "Email service down",
					"path": ["users", 0, "email"]
				}
			]`,
			expectedIndices: nil,
		},
		{
			name: "entity field is not null - should not be tainted",
			fetchReasons: []FetchReason{
				{TypeName: "Product", FieldName: "reviews", IsRequires: true, Nullable: true},
			},
			responseJSON: `[
				{"__typename": "Product", "upc": "1", "reviews": []}
			]`,
			errorsJSON: `[
				{
					"message": "Some error occurred",
					"path": ["_entities", 0, "reviews"]
				}
			]`,
			expectedIndices: nil,
		},
		{
			name: "missing __typename should not taint entity",
			fetchReasons: []FetchReason{
				{TypeName: "Product", FieldName: "reviews", IsRequires: true, Nullable: true},
			},
			responseJSON: `[
				{"upc": "1", "reviews": null}
			]`,
			errorsJSON: `[
				{
					"message": "Reviews failed",
					"path": ["_entities", 0, "reviews"]
				}
			]`,
			expectedIndices: nil,
		},
		{
			name: "deeply nested entity path",
			fetchReasons: []FetchReason{
				{TypeName: "Review", FieldName: "sentiment", IsRequires: true, Nullable: true},
			},
			responseJSON: `[
				{
					"__typename": "Product",
					"reviews": [
						{"__typename": "Review", "id": "1", "sentiment": "cool"}
					]
				},
				{
					"__typename": "Product",
					"reviews": [
						{"__typename": "Review", "id": "2", "sentiment": null}
					]
				}
			]`,
			errorsJSON: `[
				{
					"message": "Sentiment analysis failed",
					"path": ["_entities", 1, "reviews", 0, "sentiment"]
				}
			]`,
			expectedIndices: []int{1},
		},
		{
			name: "error path too short",
			fetchReasons: []FetchReason{
				{TypeName: "Product", FieldName: "name", IsRequires: true, Nullable: true},
			},
			responseJSON: `[
				{"__typename": "Product", "upc": "1", "name": null}
			]`,
			errorsJSON: `[
				{
					"message": "General error",
					"path": ["_entities", 0]
				}
			]`,
			expectedIndices: nil,
		},
		{
			name: "invalid error path format",
			fetchReasons: []FetchReason{
				{TypeName: "Product", FieldName: "reviews", IsRequires: true, Nullable: true},
			},
			responseJSON: `[
				{"__typename": "Product", "upc": "1", "reviews": null}
			]`,
			errorsJSON: `[
				{
					"message": "Invalid path",
					"path": "not_an_array"
				}
			]`,
			expectedIndices: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock fetch with FetchInfo
			fetchInfo := &FetchInfo{
				FetchReasons: tt.fetchReasons,
			}
			mockFetch := &mockFetchWithInfo{info: fetchInfo}

			response, err := astjson.ParseBytesWithoutCache([]byte(tt.responseJSON))
			assert.NoError(t, err, "Failed to parse response JSON")

			errors, err := astjson.ParseBytesWithoutCache([]byte(tt.errorsJSON))
			assert.NoError(t, err, "Failed to parse errors JSON")

			indices := getTaintedIndices(mockFetch, response, errors)
			assert.ElementsMatch(t, tt.expectedIndices, indices)
		})
	}
}

// Mock fetch implementation for testing
type mockFetchWithInfo struct {
	info *FetchInfo
}

func (m *mockFetchWithInfo) FetchInfo() *FetchInfo {
	return m.info
}

func (m *mockFetchWithInfo) FetchKind() FetchKind {
	return FetchKindSingle
}

func (m *mockFetchWithInfo) Dependencies() *FetchDependencies {
	return nil
}
