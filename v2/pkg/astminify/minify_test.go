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

func TestMinifier_MinifySmall(t *testing.T) {
	operation, err := os.ReadFile("operation.graphql")
	assert.NoError(t, err)

	schema, err := os.ReadFile("federated-schema.graphql")
	assert.NoError(t, err)

	m, err := NewMinifier(string(operation), string(schema))
	assert.NoError(t, err)
	opts := MinifyOptions{
		Pretty:    true,
		Threshold: 0,
	}
	minified, err := m.Minify(opts)
	assert.NoError(t, err)

	assert.NoError(t, err)
	goldie.Assert(t, "operation.min.graphql", []byte(minified))

	in := unsafeparser.ParseGraphqlDocumentString(string(operation))
	inPrint := unsafeprinter.Print(&in, nil)
	out := unsafeparser.ParseGraphqlDocumentString(minified)
	outPrint := unsafeprinter.Print(&out, nil)

	opts.Threshold = 12

	m, err = NewMinifier(minified, string(schema))
	assert.NoError(t, err)
	minified, err = m.Minify(opts)
	assert.NoError(t, err)

	out = unsafeparser.ParseGraphqlDocumentString(minified)
	outPrint2 := unsafeprinter.Print(&out, nil)
	goldie.Assert(t, "operation.min.min.graphql.graphql", []byte(minified))

	fmt.Printf("(run1) originalSize: %d, minifiedSize: %d, compression: %f\n", len(inPrint), len(outPrint), float64(len(outPrint))/float64(len(inPrint)))
	fmt.Printf("(run2) originalSize: %d, minifiedSize: %d, compression: %f\n", len(outPrint), len(outPrint2), float64(len(outPrint2))/float64(len(outPrint)))

}

func TestMinifier_Minify(t *testing.T) {
	operation, err := os.ReadFile("cosmo.graphql")
	assert.NoError(t, err)

	schema, err := os.ReadFile("tsb-us-in.graphql")
	assert.NoError(t, err)

	m, err := NewMinifier(string(operation), string(schema))
	assert.NoError(t, err)
	opts := MinifyOptions{
		Pretty:    true,
		Threshold: 0,
	}
	minified, err := m.Minify(opts)
	assert.NoError(t, err)

	assert.NoError(t, err)
	goldie.Assert(t, "cosmo.min.graphql", []byte(minified))

	in := unsafeparser.ParseGraphqlDocumentString(string(operation))
	inPrint := unsafeprinter.Print(&in, nil)
	out := unsafeparser.ParseGraphqlDocumentString(minified)
	outPrint := unsafeprinter.Print(&out, nil)

	opts.Threshold = 12

	m, err = NewMinifier(minified, string(schema))
	assert.NoError(t, err)
	minified, err = m.Minify(opts)
	assert.NoError(t, err)

	out = unsafeparser.ParseGraphqlDocumentString(minified)
	outPrint2 := unsafeprinter.Print(&out, nil)
	goldie.Assert(t, "cosmo.min.min.graphql", []byte(minified))

	fmt.Printf("(run1) originalSize: %d, minifiedSize: %d, compression: %f\n", len(inPrint), len(outPrint), float64(len(outPrint))/float64(len(inPrint)))
	fmt.Printf("(run2) originalSize: %d, minifiedSize: %d, compression: %f\n", len(outPrint), len(outPrint2), float64(len(outPrint2))/float64(len(outPrint)))

}

func TestMinifier_MinifyMultipleRuns(t *testing.T) {
	original, err := os.ReadFile("cosmo.graphql")
	assert.NoError(t, err)

	schema, err := os.ReadFile("tsb-us-in.graphql")
	assert.NoError(t, err)

	definition := unsafeparser.ParseGraphqlDocumentBytes(schema)
	err = asttransform.MergeDefinitionWithBaseSchema(&definition)
	assert.NoError(t, err)

	previousRun := 1

	for i := 0; i < 10; i++ {
		m, err := NewMinifier(string(original), string(schema))
		assert.NoError(t, err)
		opts := MinifyOptions{
			Pretty:    true,
			Threshold: 12,
		}
		minified, err := m.Minify(opts)
		assert.NoError(t, err)

		assert.NoError(t, err)
		goldie.Assert(t, fmt.Sprintf("cosmo.min.run-%d.graphql", i), []byte(minified))

		norm := astnormalization.NewWithOpts(
			astnormalization.WithInlineFragmentSpreads(),
		)

		report := &operationreport.Report{}

		in := unsafeparser.ParseGraphqlDocumentString(string(original))
		norm.NormalizeNamedOperation(&in, &definition, []byte(`MyQuery`), report)
		if report.HasErrors() {
			t.Fatal(report.Error())
		}

		inPrint := unsafeprinter.Print(&in, &definition)
		out := unsafeparser.ParseGraphqlDocumentString(minified)
		outPrint := unsafeprinter.Print(&out, &definition)

		norm.NormalizeNamedOperation(&out, &definition, []byte(`MyQuery`), report)
		if report.HasErrors() {
			t.Fatal(report.Error())
		}

		outPrintNormalized := unsafeprinter.Print(&out, &definition)
		assert.Equal(t, outPrint, outPrintNormalized)

		if previousRun != 1 && len(outPrint) >= previousRun {
			fmt.Printf("(run: %d) no more compression possible\n", i)
			break
		}

		fmt.Printf("(run: %d) originalSize: %d, minifiedSize: %d, compression: %f improvement: %f\n", i, len(inPrint), len(outPrint), float64(len(outPrint))/float64(len(inPrint)), float64(len(outPrint))/float64(previousRun))
		previousRun = len(outPrint)
	}
}
