package astminify

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/testing/goldie"
)

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
