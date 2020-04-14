package asttransform

import (
	"bytes"
	"github.com/jensneuse/diffview"
	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
	"github.com/sebdah/goldie"
	"io/ioutil"
	"testing"
)

func runTestMerge(definition, fixtureName string) func(t *testing.T) {
	return func(t *testing.T) {
		doc := unsafeparser.ParseGraphqlDocumentString(definition)
		err := MergeDefinitionWithBaseSchema(&doc)
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
			want, err := ioutil.ReadFile("./fixtures/" + fixtureName + ".golden")
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
}
