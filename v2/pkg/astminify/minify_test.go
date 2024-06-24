package astminify

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/testing/goldie"
)

func BenchmarkMinify(b *testing.B) {
	operation, err := os.ReadFile("cosmo.graphql")
	assert.NoError(b, err)

	schema, err := os.ReadFile("tsb-us-in.graphql")
	assert.NoError(b, err)

	definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(string(schema))

	opts := MinifyOptions{
		SortAST: true,
	}

	buf := &bytes.Buffer{}
	m := NewMinifier()

	b.ReportAllocs()
	b.SetBytes(int64(len(operation)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		buf.Reset()
		err := m.Minify(operation, &definition, opts, buf)
		if err != nil {
			b.Fatal(err)
		}
		if buf.Len() != 10366 {
			b.Fatalf("unexpected length: %d, run: %d", buf.Len(), i)
		}
	}
}

type testCase struct {
	name          string
	operationFile string
	operationName string
	schemaFile    string
	sort          bool
}

func TestMinifier_Minify(t *testing.T) {
	testCases := []testCase{
		{
			name:          "operation1",
			operationFile: "operation1.graphql",
			operationName: "MyQuery",
			schemaFile:    "simpleschema.graphql",
			sort:          true,
		},
		{
			name:          "operation2",
			operationFile: "operation2.graphql",
			operationName: "MyQuery",
			schemaFile:    "simpleschema.graphql",
			sort:          true,
		},
		{
			name:          "operation3",
			operationFile: "operation3.graphql",
			operationName: "MyQuery",
			schemaFile:    "simpleschema.graphql",
			sort:          true,
		},
		{
			name:          "operation4",
			operationFile: "operation4.graphql",
			operationName: "MyQuery",
			schemaFile:    "simpleschema.graphql",
			sort:          true,
		},
		{
			name:          "operation5",
			operationFile: "operation5.graphql",
			operationName: "MyQuery",
			schemaFile:    "simpleschema.graphql",
			sort:          true,
		},
	}

	if os.Getenv("WG_INTERNAL") == "true" {
		testCases = append(testCases, testCase{
			name:          "cosmo-presorted",
			operationFile: "cosmo-sorted.graphql",
			operationName: "MyQuery",
			schemaFile:    "tsb-us-in.graphql",
			sort:          true,
		},
			testCase{
				name:          "cosmo-nosort",
				operationFile: "cosmo.graphql",
				operationName: "MyQuery",
				schemaFile:    "tsb-us-in.graphql",
				sort:          false,
			},
			testCase{
				name:          "cosmo-sorted",
				operationFile: "cosmo.graphql",
				operationName: "MyQuery",
				schemaFile:    "tsb-us-in.graphql",
				sort:          true,
			},
		)
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			operation, err := os.ReadFile(tc.operationFile)
			assert.NoError(t, err)

			schema, err := os.ReadFile(tc.schemaFile)
			assert.NoError(t, err)

			m := NewMinifier()
			assert.NoError(t, err)
			opts := MinifyOptions{
				Pretty: true,
			}
			definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(string(schema))
			buf := &bytes.Buffer{}
			err = m.Minify(operation, &definition, opts, buf)
			assert.NoError(t, err)
			minified := buf.String()

			assert.NoError(t, err)

			out := unsafeparser.ParseGraphqlDocumentString(minified)
			outPrint := unsafeprinter.Print(&out, nil)

			def := unsafeparser.ParseGraphqlDocumentString(string(schema))
			err = asttransform.MergeDefinitionWithBaseSchema(&def)
			assert.NoError(t, err)

			best := unsafeparser.ParseGraphqlDocumentString(outPrint)
			normalizer := astnormalization.NewWithOpts(
				astnormalization.WithExtractVariables(),
				astnormalization.WithInlineFragmentSpreads(),
				astnormalization.WithRemoveFragmentDefinitions(),
				astnormalization.WithRemoveNotMatchingOperationDefinitions(),
				astnormalization.WithRemoveUnusedVariables(),
			)
			rep := &operationreport.Report{}
			normalizer.NormalizeNamedOperation(&best, &def, []byte(tc.operationName), rep)
			if rep.HasErrors() {
				t.Fatal(rep.Error())
			}
			bestNormalized := unsafeprinter.PrettyPrint(&best, &def)

			orig := unsafeparser.ParseGraphqlDocumentString(string(operation))
			normalizer.NormalizeNamedOperation(&orig, &def, []byte(tc.operationName), rep)
			if rep.HasErrors() {
				t.Fatal(rep.Error())
			}
			origNormalized := unsafeprinter.PrettyPrint(&orig, &def)

			if !tc.sort {
				assert.Equal(t, origNormalized, bestNormalized)
			}
			goldie.Assert(t, fmt.Sprintf("%s.min.graphql", tc.name), []byte(minified))
			goldie.Assert(t, fmt.Sprintf("%s.min.normalized.graphql", tc.name), []byte(bestNormalized))
			goldie.Assert(t, fmt.Sprintf("%s.normalized.graphql", tc.name), []byte(origNormalized))
			fmt.Printf("originalSize: %d, minifiedSize: %d, compression: %f\n", len(operation), len(minified), float64(len(minified))/float64(len(operation)))
		})
	}
}
