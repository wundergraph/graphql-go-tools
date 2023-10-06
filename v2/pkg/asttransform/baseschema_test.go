package asttransform_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/jensneuse/diffview"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/testing/goldie"
)

func runTestMerge(definition, fixtureName string) func(t *testing.T) {
	return func(t *testing.T) {
		doc := unsafeparser.ParseGraphqlDocumentString(definition)
		err := asttransform.MergeDefinitionWithBaseSchema(&doc)
		if err != nil {
			panic(err)
		}
		buf := bytes.Buffer{}
		err = astprinter.PrintIndent(&doc, nil, []byte("  "), &buf)
		if err != nil {
			panic(err)
		}
		got := buf.Bytes()
		goldie.Assert(t, fixtureName, got)
		if t.Failed() {
			want, err := os.ReadFile("./fixtures/" + fixtureName + ".golden")
			if err != nil {
				panic(err)
			}
			diffview.NewGoland().DiffViewBytes(fixtureName, want, got)
		}
	}
}

func TestMergeDefinitionWithBaseSchema(t *testing.T) {
	t.Run("simple", runTestMerge(`
			schema {
				query: Query
			}
			type Query {
				hello(name: String): Hello!
			}
			type Hello {
				hello: String!
				object: String!
				adminInformation: String!
			}
	`, "simple"))
	t.Run("mutation only", runTestMerge(`
			type Mutation {
				m: String!
			}
	`, "mutation_only"))
	t.Run("schema with mutation", runTestMerge(`
			schema {
				mutation: Mutation
			}
			type Mutation {
				m: String!
			}
	`, "mutation_only"))

	t.Run("subscription only", runTestMerge(`
			type Subscription {
				s: String!
			}
	`, "subscription_only"))
	t.Run("schema with subscription", runTestMerge(`
			schema {
				subscription: Subscription
			}
			type Subscription {
				s: String!
			}
	`, "subscription_only"))
	t.Run("schema with renamed subscription", runTestMerge(`
			schema {
				subscription: Sub
			}
			type Sub {
				s: String!
			}
	`, "subscription_renamed"))

	t.Run("schema missing", runTestMerge(`
			type Query {
				hello(name: String): Hello!
			}
			type Hello {
				hello: String!
				object: String!
				adminInformation: String!
			}
	`, "schema_missing"))
	t.Run("custom query type name", runTestMerge(`
			schema {
				query: query_root
			}
			type query_root {
				hello(name: String): Hello!
			}
			type Hello {
				hello: String!
				object: String!
				adminInformation: String!
			}
	`, "custom_query_name"))
	t.Run("complete", runTestMerge(`
			schema {
				query: Query
			}
			type Query {
				hello(name: String): Hello!
				__schema: __Schema!
				__type(name: String!): __Type
			}
			type Hello {
				hello: String!
				object: String!
				adminInformation: String!
			}
	`, "complete"))
	t.Run("with mutation & subscription", runTestMerge(`
			schema {
				query: Query
				mutation: Mutation
				subscription: Subscription
			}
			type Mutation {
				m: String!
			}
			type Subscription {
				s: String!
			}
			type Query {
				hello(name: String): Hello!
				__schema: __Schema!
				__type(name: String!): __Type
			}
			type Hello {
				hello: String!
				object: String!
				adminInformation: String!
			}
	`, "with_mutation_subscription"))
}
