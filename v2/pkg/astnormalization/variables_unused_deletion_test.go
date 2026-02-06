package astnormalization

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestVariablesUnusedDeletion(t *testing.T) {
	t.Parallel()
	input := `
		mutation HttpBinPost($foo: String! $bar: String!){
		  httpBinPost(input: {foo: $foo}){
			headers {
			  userAgent
			}
			data {
			  foo
			}
		  }
		}
		`

	definitionDocument := unsafeparser.ParseGraphqlDocumentString(variablesExtractionDefinition)
	err := asttransform.MergeDefinitionWithBaseSchema(&definitionDocument)
	if err != nil {
		panic(err)
	}

	operationDocument := unsafeparser.ParseGraphqlDocumentString(input)
	operationDocument.Input.Variables = []byte(`{"foo":"bar"}`)

	firstWalker := astvisitor.NewWalker(8)
	secondWalker := astvisitor.NewWalker(8)

	del := deleteUnusedVariables(&secondWalker)
	detectVariableUsage(&firstWalker, del)
	extractVariables(&firstWalker, false)

	rep := &operationreport.Report{}
	firstWalker.Walk(&operationDocument, &definitionDocument, rep)
	require.False(t, rep.HasErrors())
	secondWalker.Walk(&operationDocument, &definitionDocument, rep)
	require.False(t, rep.HasErrors())

	out := unsafeprinter.Print(&operationDocument)
	require.Equal(t, `mutation HttpBinPost($bar: String!, $a: HttpBinPostInput){httpBinPost(input: $a){headers {userAgent} data {foo}}}`, out)
	require.Equal(t, `{"a":{"foo":"bar"}}`, string(operationDocument.Input.Variables))
}
