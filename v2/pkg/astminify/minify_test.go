package astminify

import (
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

type testCase struct {
	name          string
	operationFile string
	operationName string
	schemaFile    string
}

func TestMinifier_Minify(t *testing.T) {
	testCases := []testCase{
		{
			name:          "operation1",
			operationFile: "operation1.graphql",
			operationName: "MyQuery",
			schemaFile:    "simpleschema.graphql",
		},
		{
			name:          "operation2",
			operationFile: "operation2.graphql",
			operationName: "MyQuery",
			schemaFile:    "simpleschema.graphql",
		},
		{
			name:          "operation3",
			operationFile: "operation3.graphql",
			operationName: "MyQuery",
			schemaFile:    "simpleschema.graphql",
		},
	}

	if os.Getenv("WG_INTERNAL") == "true" {
		testCases = append(testCases, testCase{
			name:          "cosmo-sorted",
			operationFile: "cosmo-sorted.graphql",
			operationName: "MyQuery",
			schemaFile:    "tsb-us-in.graphql",
		})
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			operation, err := os.ReadFile(tc.operationFile)
			assert.NoError(t, err)

			schema, err := os.ReadFile(tc.schemaFile)
			assert.NoError(t, err)

			m, err := NewMinifier(string(operation), string(schema))
			assert.NoError(t, err)
			opts := MinifyOptions{
				Pretty: true,
			}
			minified, err := m.Minify(opts)
			assert.NoError(t, err)

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

			assert.Equal(t, origNormalized, bestNormalized)
			goldie.Assert(t, fmt.Sprintf("%s.min.graphql", tc.name), []byte(minified))
			goldie.Assert(t, fmt.Sprintf("%s.min.normalized.graphql", tc.name), []byte(bestNormalized))
			goldie.Assert(t, fmt.Sprintf("%s.normalized.graphql", tc.name), []byte(origNormalized))
			fmt.Printf("originalSize: %d, minifiedSize: %d, compression: %f\n", len(operation), len(minified), float64(len(minified))/float64(len(operation)))
		})
	}
}
