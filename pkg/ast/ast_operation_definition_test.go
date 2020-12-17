package ast

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDocument_OperationNameExists(t *testing.T) {
	run := func(operationDefinitionsFunc func(doc *Document) []OperationDefinition, operationName string, expectedExists bool) func(t *testing.T) {
		return func(t *testing.T) {
			doc := Document{}
			operationDefinitions := operationDefinitionsFunc(&doc)
			doc.OperationDefinitions = append(doc.OperationDefinitions, operationDefinitions...)
			for i := range doc.OperationDefinitions {
				doc.RootNodes = append(doc.RootNodes, Node{
					Kind: NodeKindOperationDefinition,
					Ref: i,
				})
			}
			exists := doc.OperationNameExists(operationName)
			assert.Equal(t, expectedExists, exists)
		}
	}

	t.Run("not found on empty document", run(
		func(doc *Document) []OperationDefinition {
			return []OperationDefinition{}
		},
		"MyOperation",
		false,
	))

	t.Run("not found on document with multiple operations", run(
		func(doc *Document) []OperationDefinition {
			return []OperationDefinition{
				{
					Name: doc.Input.AppendInputString("OtherOperation"),
				},
				{
					Name: doc.Input.AppendInputString("AnotherOperation"),
				},
			}
		},
		"MyOperation",
		false,
	))

	t.Run("found on document with a single operations", run(
		func(doc *Document) []OperationDefinition {
			return []OperationDefinition{
				{
					Name: doc.Input.AppendInputString("MyOperation"),
				},
			}
		},
		"MyOperation",
		true,
	))

	t.Run("found on document with multiple operations", run(
		func(doc *Document) []OperationDefinition {
			return []OperationDefinition{
				{
					Name: doc.Input.AppendInputString("OtherOperation"),
				},
				{
					Name: doc.Input.AppendInputString("MyOperation"),
				},
				{
					Name: doc.Input.AppendInputString("AnotherOperation"),
				},
			}
		},
		"MyOperation",
		true,
	))
}
