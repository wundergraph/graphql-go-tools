package postprocess

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestCacheKeySpecFreezerFreezeEntity(t *testing.T) {
	tests := []struct {
		name       string
		definition string
		typeName   string
		keys       []plan.FederationFieldConfiguration
		expected   resolve.CacheKeySpec
		expectedOK bool
	}{
		{
			name: "single key",
			definition: `
				scalar String

				type User {
					id: String!
				}
			`,
			typeName: "User",
			keys: []plan.FederationFieldConfiguration{
				{TypeName: "User", SelectionSet: "id"},
			},
			expected: resolve.CacheKeySpec{
				Scope:     resolve.CacheScopeEntity,
				TypeName:  "User",
				FieldName: "_entities",
				Candidates: []resolve.CacheKeyCandidate{
					{Representation: entityKeyObject("User", scalarKeyField("id"))},
				},
			},
			expectedOK: true,
		},
		{
			name: "composite key",
			definition: `
				scalar String

				type User {
					a: String!
					b: String!
				}
			`,
			typeName: "User",
			keys: []plan.FederationFieldConfiguration{
				{TypeName: "User", SelectionSet: "a b"},
			},
			expected: resolve.CacheKeySpec{
				Scope:     resolve.CacheScopeEntity,
				TypeName:  "User",
				FieldName: "_entities",
				Candidates: []resolve.CacheKeyCandidate{
					{Representation: entityKeyObject("User", scalarKeyField("a"), scalarKeyField("b"))},
				},
			},
			expectedOK: true,
		},
		{
			name: "nested object key",
			definition: `
				scalar String

				type User {
					info: UserInfo!
				}

				type UserInfo {
					id: String!
				}
			`,
			typeName: "User",
			keys: []plan.FederationFieldConfiguration{
				{TypeName: "User", SelectionSet: "info { id }"},
			},
			expected: resolve.CacheKeySpec{
				Scope:     resolve.CacheScopeEntity,
				TypeName:  "User",
				FieldName: "_entities",
				Candidates: []resolve.CacheKeyCandidate{
					{Representation: entityKeyObject("User", objectKeyField("info", scalarKeyField("id")))},
				},
			},
			expectedOK: true,
		},
		{
			name: "multiple keys are sorted by selection set",
			definition: `
				scalar String

				type Product {
					upc: String!
					sku: String!
				}
			`,
			typeName: "Product",
			keys: []plan.FederationFieldConfiguration{
				{TypeName: "Product", SelectionSet: "upc"},
				{TypeName: "Product", SelectionSet: "sku"},
			},
			expected: resolve.CacheKeySpec{
				Scope:     resolve.CacheScopeEntity,
				TypeName:  "Product",
				FieldName: "_entities",
				Candidates: []resolve.CacheKeyCandidate{
					{Representation: entityKeyObject("Product", scalarKeyField("sku"))},
					{Representation: entityKeyObject("Product", scalarKeyField("upc"))},
				},
			},
			expectedOK: true,
		},
		{
			name: "no key type",
			definition: `
				scalar String

				type Review {
					id: String!
				}
			`,
			typeName:   "Review",
			keys:       nil,
			expected:   resolve.CacheKeySpec{},
			expectedOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			definition := parseFreezerDefinition(t, tt.definition)
			federation := initFreezerFederation(t, tt.keys)
			freezer := &cacheKeySpecFreezer{
				federation: map[string]plan.FederationMetaData{"ds": federation},
				definition: definition,
			}
			info := &resolve.FetchInfo{
				DataSourceID: "ds",
				RootFields: []resolve.GraphCoordinate{
					{TypeName: tt.typeName, FieldName: "_entities"},
				},
			}

			spec, ok := freezer.freeze(resolve.CacheScopeEntity, info)

			assert.Equal(t, tt.expectedOK, ok)
			assert.Equal(t, tt.expected, spec)

			federation.Keys = append(federation.Keys, plan.FederationFieldConfiguration{TypeName: tt.typeName, SelectionSet: "mutated"})
			freezer.federation["ds"] = federation
			assert.Equal(t, tt.expected, spec)
		})
	}
}

func TestCacheKeySpecFreezerFreezeRootField(t *testing.T) {
	definition := parseFreezerDefinition(t, `
		scalar String

		type Query {
			product(upc: String!): Product
			user(id: String!): User
			review(slug: String!): Review
			topProducts: [Product!]!
		}

		type Product {
			upc: String!
			sku: String!
			name: String!
		}

		type User {
			id: String!
			name: String!
		}

		type Review {
			id: String!
			slug: String!
		}
	`)
	federation := initFreezerFederation(t, []plan.FederationFieldConfiguration{
		{TypeName: "Product", SelectionSet: "upc"},
		{TypeName: "Product", SelectionSet: "sku"},
		{TypeName: "User", SelectionSet: "id"},
		{TypeName: "Review", SelectionSet: "id"},
	})
	freezer := &cacheKeySpecFreezer{
		federation: map[string]plan.FederationMetaData{"ds": federation},
		definition: definition,
	}
	tests := []struct {
		name     string
		field    string
		expected resolve.CacheKeySpec
	}{
		{
			name:  "product arg matches one product key",
			field: "product",
			expected: resolve.CacheKeySpec{
				Scope:     resolve.CacheScopeRootField,
				TypeName:  "Query",
				FieldName: "product",
				EntityKeyMappings: []resolve.EntityKeyMapping{
					{
						EntityTypeName: "Product",
						FieldMappings: []resolve.EntityFieldMapping{
							{EntityKeyField: "upc", ArgumentPath: []string{"upc"}, ArgumentIsEntityKey: true},
						},
					},
				},
			},
		},
		{
			name:  "user arg matches user key",
			field: "user",
			expected: resolve.CacheKeySpec{
				Scope:     resolve.CacheScopeRootField,
				TypeName:  "Query",
				FieldName: "user",
				EntityKeyMappings: []resolve.EntityKeyMapping{
					{
						EntityTypeName: "User",
						FieldMappings: []resolve.EntityFieldMapping{
							{EntityKeyField: "id", ArgumentPath: []string{"id"}, ArgumentIsEntityKey: true},
						},
					},
				},
			},
		},
		{
			name:  "root args do not match entity key",
			field: "review",
			expected: resolve.CacheKeySpec{
				Scope:             resolve.CacheScopeRootField,
				TypeName:          "Query",
				FieldName:         "review",
				EntityKeyMappings: []resolve.EntityKeyMapping{},
			},
		},
		{
			name:  "root field without args has no mapping",
			field: "topProducts",
			expected: resolve.CacheKeySpec{
				Scope:     resolve.CacheScopeRootField,
				TypeName:  "Query",
				FieldName: "topProducts",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &resolve.FetchInfo{
				DataSourceID: "ds",
				RootFields: []resolve.GraphCoordinate{
					{TypeName: "Query", FieldName: tt.field},
				},
			}

			spec, ok := freezer.freeze(resolve.CacheScopeRootField, info)

			assert.Equal(t, true, ok)
			assert.Equal(t, tt.expected, spec)

			federation.Keys = append(federation.Keys, plan.FederationFieldConfiguration{TypeName: "Product", SelectionSet: "mutated"})
			freezer.federation["ds"] = federation
			assert.Equal(t, tt.expected, spec)
		})
	}
}

func parseFreezerDefinition(t *testing.T, input string) *ast.Document {
	t.Helper()

	definition, report := astparser.ParseGraphqlDocumentString(input)
	require.False(t, report.HasErrors(), report.Error())
	return &definition
}

func initFreezerFederation(t *testing.T, keys []plan.FederationFieldConfiguration) plan.FederationMetaData {
	t.Helper()

	metadata := plan.DataSourceMetadata{
		FederationMetaData: plan.FederationMetaData{
			Keys: keys,
		},
	}
	require.NoError(t, metadata.Init())
	return metadata.FederationMetaData
}

func entityKeyObject(typeName string, fields ...*resolve.Field) *resolve.Object {
	for _, field := range fields {
		if field.OnTypeNames == nil {
			field.OnTypeNames = [][]byte{[]byte(typeName)}
		}
	}
	allFields := []*resolve.Field{
		{
			Name: []byte("__typename"),
			Value: &resolve.String{
				Path: []string{"__typename"},
			},
			OnTypeNames: [][]byte{[]byte(typeName)},
		},
	}
	allFields = append(allFields, fields...)
	return &resolve.Object{
		Nullable: true,
		Fields:   allFields,
	}
}

func scalarKeyField(name string) *resolve.Field {
	return &resolve.Field{
		Name: []byte(name),
		Value: &resolve.String{
			Path: []string{name},
		},
	}
}

func objectKeyField(name string, fields ...*resolve.Field) *resolve.Field {
	return &resolve.Field{
		Name: []byte(name),
		Value: &resolve.Object{
			Path:   []string{name},
			Fields: fields,
		},
	}
}
