package introspection

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
)

func TestJSONConverter_GraphQLDocument(t *testing.T) {
	fixture, err := ioutil.ReadFile("./fixtures/startwars_introspected.golden")
	assert.NoError(t, err)
	buf := bytes.NewBuffer(fixture)

	converter := JsonConverter{}
	doc, err := converter.GraphQLDocument(buf)
	assert.NoError(t, err)

	printDoc(doc)
}

func printDoc(doc *ast.Document) {
	outWriter := &bytes.Buffer{}
	err := astprinter.PrintIndent(doc, nil, []byte("  "), outWriter)
	if err != nil {
		panic(err)
	}

	fmt.Println(outWriter.String())
}
