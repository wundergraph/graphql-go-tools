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

func TestMinifier_MinifySimple1(t *testing.T) {
	operation, err := os.ReadFile("operation1.graphql")
	assert.NoError(t, err)

	schema, err := os.ReadFile("simpleschema.graphql")
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
	goldie.Assert(t, "operation1.min.graphql", []byte(minified))

	in := unsafeparser.ParseGraphqlDocumentString(string(operation))
	inPrint := unsafeprinter.Print(&in, nil)
	out := unsafeparser.ParseGraphqlDocumentString(minified)
	outPrint := unsafeprinter.Print(&out, nil)

	fmt.Printf("(run1) originalSize: %d, minifiedSize: %d, compression: %f\n", len(inPrint), len(outPrint), float64(len(outPrint))/float64(len(inPrint)))
}

func TestMinifier_MinifyCosmoSorted(t *testing.T) {
	operation, err := os.ReadFile("cosmo-sorted.graphql")
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
	goldie.Assert(t, "cosmo-sorted.min.graphql", []byte(minified))

	in := unsafeparser.ParseGraphqlDocumentString(string(operation))
	inPrint := unsafeprinter.Print(&in, nil)
	out := unsafeparser.ParseGraphqlDocumentString(minified)
	outPrint := unsafeprinter.Print(&out, nil)

	fmt.Printf("(run1) originalSize: %d, minifiedSize: %d, compression: %f\n", len(inPrint), len(outPrint), float64(len(outPrint))/float64(len(inPrint)))
}

func TestMinifier_Minify(t *testing.T) {

	operation, err := os.ReadFile("cosmo-sorted.graphql")
	assert.NoError(t, err)

	schema, err := os.ReadFile("tsb-us-in.graphql")
	assert.NoError(t, err)

	runs := 1000

	bestCompression := 1.0
	bestMinified := ""
	in := unsafeparser.ParseGraphqlDocumentString(string(operation))
	inPrint := unsafeprinter.Print(&in, nil)

	for i := 0; i < runs; i++ {
		m, err := NewMinifier(string(operation), string(schema))
		assert.NoError(t, err)
		opts := MinifyOptions{
			Pretty:    true,
			Threshold: 0,
		}
		minified, err := m.Minify(opts)
		assert.NoError(t, err)

		assert.NoError(t, err)

		out := unsafeparser.ParseGraphqlDocumentString(minified)
		outPrint := unsafeprinter.Print(&out, nil)

		//fmt.Printf("(run:%d) originalSize: %d, minifiedSize: %d, compression: %f\n", i, len(inPrint), len(outPrint), float64(len(outPrint))/float64(len(inPrint)))

		if float64(len(outPrint))/float64(len(inPrint)) < bestCompression {
			bestCompression = float64(len(outPrint)) / float64(len(inPrint))
			bestMinified = minified
		}
	}

	def := unsafeparser.ParseGraphqlDocumentString(string(schema))
	err = asttransform.MergeDefinitionWithBaseSchema(&def)
	assert.NoError(t, err)

	best := unsafeparser.ParseGraphqlDocumentString(string(bestMinified))
	normalizer := astnormalization.NewWithOpts(
		astnormalization.WithExtractVariables(),
		astnormalization.WithInlineFragmentSpreads(),
		astnormalization.WithRemoveFragmentDefinitions(),
		astnormalization.WithRemoveNotMatchingOperationDefinitions(),
		astnormalization.WithRemoveUnusedVariables(),
	)
	rep := &operationreport.Report{}
	normalizer.NormalizeNamedOperation(&best, &def, []byte(`MyQuery`), rep)
	if rep.HasErrors() {
		t.Fatal(rep.Error())
	}
	bestNormalized := unsafeprinter.PrettyPrint(&best, &def)

	orig := unsafeparser.ParseGraphqlDocumentString(string(operation))
	normalizer.NormalizeNamedOperation(&orig, &def, []byte(`MyQuery`), rep)
	if rep.HasErrors() {
		t.Fatal(rep.Error())
	}
	origNormalized := unsafeprinter.PrettyPrint(&orig, &def)

	goldie.Assert(t, "cosmo-sorted.min.graphql", []byte(bestMinified))
	goldie.Assert(t, "cosmo-sorted.min.normalized.graphql", []byte(bestNormalized))
	goldie.Assert(t, "cosmo-sorted.normalized.graphql", []byte(origNormalized))
	fmt.Printf("\n\nbest run - originalSize: %d, minifiedSize: %d, compression: %f\n", len(inPrint), len(bestMinified), bestCompression)
}

func TestMinifier_MinifyMultipleRuns(t *testing.T) {
	original, err := os.ReadFile("cosmo-sorted.graphql")
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
