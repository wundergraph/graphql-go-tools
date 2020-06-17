package graphql

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSchemaFromReader(t *testing.T) {
	t.Run("should return error when an error occures internally", func(t *testing.T) {
		schemaBytes := []byte("query: Query")
		schemaReader := bytes.NewBuffer(schemaBytes)
		schema, err := NewSchemaFromReader(schemaReader)

		assert.Error(t, err)
		assert.Nil(t, schema)
	})

	t.Run("should successfully read from io.Reader", func(t *testing.T) {
		schemaBytes := []byte("schema { query: Query } type Query { hello: String }")
		schemaReader := bytes.NewBuffer(schemaBytes)
		schema, err := NewSchemaFromReader(schemaReader)

		assert.NoError(t, err)
		assert.Equal(t, schemaBytes, schema.rawInput)
	})
}

func TestNewSchemaFromString(t *testing.T) {
	t.Run("should return error when an error occures internally", func(t *testing.T) {
		schemaBytes := []byte("query: Query")
		schema, err := NewSchemaFromString(string(schemaBytes))

		assert.Error(t, err)
		assert.Nil(t, schema)
	})

	t.Run("should successfully read from string", func(t *testing.T) {
		schemaBytes := []byte("schema { query: Query } type Query { hello: String }")
		schema, err := NewSchemaFromString(string(schemaBytes))

		assert.NoError(t, err)
		assert.Equal(t, schemaBytes, schema.rawInput)
	})
}

func TestSchema_HasQueryType(t *testing.T) {
	run := func(schema string, expectation bool) func(t *testing.T) {
		return func(t *testing.T) {
			parsedSchema, err := NewSchemaFromString(schema)
			require.NoError(t, err)

			result := parsedSchema.HasQueryType()
			assert.Equal(t, expectation, result)
		}
	}

	t.Run("should return false when there is no query type present", run(`
				schema {
					mutation: Mutation
				}
				type Mutation {
					save: Boolean!
				}`, false),
	)

	t.Run("should return true when there is a query type present", run(`
				schema {
					query: Query
				}
				type Query {
					hello: String!
				}`, true),
	)
}

func TestSchema_QueryTypeName(t *testing.T) {
	run := func(schema string, expectation string) func(t *testing.T) {
		return func(t *testing.T) {
			parsedSchema, err := NewSchemaFromString(schema)
			require.NoError(t, err)

			result := parsedSchema.QueryTypeName()
			assert.Equal(t, expectation, result)
		}
	}

	t.Run("should return empty string when no query type is present", run(`
				schema {
					mutation: Mutation
				}
				type Mutation {
					save: Boolean!
				}`, ""),
	)

	t.Run("should return 'Query' when there is a query type named 'Query'", run(`
				schema {
					query: Query
				}
				type Query {
					hello: String!
				}`, "Query"),
	)

	t.Run("should return 'Other' when there is a query type named 'Other'", run(`
				schema {
					query: Other
				}
				type Other {
					hello: String!
				}`, "Other"),
	)
}

func TestSchema_HasMutationType(t *testing.T) {
	run := func(schema string, expectation bool) func(t *testing.T) {
		return func(t *testing.T) {
			parsedSchema, err := NewSchemaFromString(schema)
			require.NoError(t, err)

			result := parsedSchema.HasMutationType()
			assert.Equal(t, expectation, result)
		}
	}

	t.Run("should return false when there is no mutation type present", run(`
				schema {
					query: Query
				}
				type Query {
					hello: String!
				}`, false),
	)

	t.Run("should return true when there is a mutation type present", run(`
				schema {
					mutation: Mutation
				}
				type Mutation {
					save: Boolean!
				}`, true),
	)
}

func TestSchema_MutationTypeName(t *testing.T) {
	run := func(schema string, expectation string) func(t *testing.T) {
		return func(t *testing.T) {
			parsedSchema, err := NewSchemaFromString(schema)
			require.NoError(t, err)

			result := parsedSchema.MutationTypeName()
			assert.Equal(t, expectation, result)
		}
	}

	t.Run("should return empty string when no mutation type is present", run(`
				schema {
					query: Query
				}
				type Query {
					hello: String!
				}`, ""),
	)

	t.Run("should return 'Mutation' when there is a mutation type named 'Mutation'", run(`
				schema {
					mutation: Mutation
				}
				type Mutation {
					save: Boolean!
				}`, "Mutation"),
	)

	t.Run("should return 'Other' when there is a mutation type named 'Other'", run(`
				schema {
					mutation: Other
				}
				type Other {
					save: Boolean!
				}`, "Other"),
	)
}

func TestSchema_HasSubscriptionType(t *testing.T) {
	run := func(schema string, expectation bool) func(t *testing.T) {
		return func(t *testing.T) {
			parsedSchema, err := NewSchemaFromString(schema)
			require.NoError(t, err)

			result := parsedSchema.HasSubscriptionType()
			assert.Equal(t, expectation, result)
		}
	}

	t.Run("should return false when there is no subscription type present", run(`
				schema {
					query: Query
				}
				type Query {
					hello: String!
				}`, false),
	)

	t.Run("should return true when there is a subscription type present", run(`
				schema {
					subscription: Subscription
				}
				type Subscription {
					news: String!
				}`, true),
	)
}

func TestSchema_SubscriptionTypeName(t *testing.T) {
	run := func(schema string, expectation string) func(t *testing.T) {
		return func(t *testing.T) {
			parsedSchema, err := NewSchemaFromString(schema)
			require.NoError(t, err)

			result := parsedSchema.SubscriptionTypeName()
			assert.Equal(t, expectation, result)
		}
	}

	t.Run("should return empty string when no subscription type is present", run(`
				schema {
					query: Query
				}
				type Query {
					hello: String!
				}`, ""),
	)

	t.Run("should return 'Subscription' when there is a subscription type named 'Subscription'", run(`
				schema {
					subscription: Subscription
				}
				type Subscription {
					news: String!
				}`, "Subscription"),
	)

	t.Run("should return 'Other' when there is a subscription type named 'Other'", run(`
				schema {
					subscription: Other
				}
				type Other {
					news: String!
				}`, "Other"),
	)
}

func TestSchema_Document(t *testing.T) {
	schemaBytes := []byte("schema { query: Query } type Query { hello: String }")
	schema, err := NewSchemaFromString(string(schemaBytes))
	require.NoError(t, err)

	assert.Equal(t, schemaBytes, schema.Document())
}
