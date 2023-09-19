package sdlmerge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestCollectEntities(t *testing.T) {
	t.Run("Valid entities are collected", func(t *testing.T) {
		collectEntities(t, newCollectEntitiesVisitor(newTestNormalizer(false)), `
			type Dog @key(fields: "name") @key(fields: "id") {
				id: ID!
				name: String!
			}

			type Cat @key(fields: "species") {
				id: ID!
				species: String!
			}
		`, entitySet{
			"Dog": {},
			"Cat": {},
		})
	})

	t.Run("Valid entities are collected", func(t *testing.T) {
		collectEntitiesAndExpectError(t, newCollectEntitiesVisitor(newTestNormalizer(false)), `
			type Dog @key(fields: "name") @key(fields: "id") {
				id: ID!
				name: String!
			}

			type Dog @key(fields: "name") @key(fields: "id") {
				id: ID!
				name: String!
			}

			type Cat @key(fields: "species") {
				id: ID!
				species: String!
			}
		`, duplicateEntityErrorMessage("Dog"))
	})
}

var collectEntities = func(t *testing.T, visitor *collectEntitiesVisitor, operation string, expectedEntities entitySet) {
	operationDocument := unsafeparser.ParseGraphqlDocumentString(operation)
	report := operationreport.Report{}
	walker := astvisitor.NewWalker(48)

	visitor.Register(&walker)

	walker.Walk(&operationDocument, nil, &report)

	if report.HasErrors() {
		t.Fatal(report.Error())
	}

	got := visitor.collectedEntities

	assert.Equal(t, expectedEntities, got)
}

var collectEntitiesAndExpectError = func(t *testing.T, visitor *collectEntitiesVisitor, operation string, expectedError string) {
	operationDocument := unsafeparser.ParseGraphqlDocumentString(operation)
	report := operationreport.Report{}
	walker := astvisitor.NewWalker(48)

	visitor.Register(&walker)

	walker.Walk(&operationDocument, nil, &report)

	var got string
	if report.HasErrors() {
		if report.InternalErrors == nil {
			got = report.ExternalErrors[0].Message
		} else {
			got = report.InternalErrors[0].Error()
		}
	}

	assert.Equal(t, expectedError, got)
}
