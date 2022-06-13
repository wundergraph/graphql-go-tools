package sdlmerge

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCollectValidEntities(t *testing.T) {
	t.Run("Valid entities are collected", func(t *testing.T) {
		collectEntities(t, newCollectValidEntitiesVisitor(newTestNormalizer(false)), `
			type Dog @key(fields: "name") @key(fields: "id") {
				id: ID!
				name: String!
			}

			type Cat @key(fields: "species") {
				id: ID!
				species: String!
			}
		`, entitySet{
			"Dog": primaryKeySet{"name": true, "id": true},
			"Cat": primaryKeySet{"species": true},
		})
	})

	t.Run("A primary key whose field returns an interface returns an error", func(t *testing.T) {
		collectEntitiesAndExpectError(t, newCollectValidEntitiesVisitor(newTestNormalizer(false)), `
			interface Mammal {
				name: String!
			}

			type Dog @key(fields: "class") {
				class: Mammal!
			}
		`, invalidPrimaryKeyTypeErrorMessage("Dog", "class"))
	})

	t.Run("A primary key whose field returns a union returns an error", func(t *testing.T) {
		collectEntitiesAndExpectError(t, newCollectValidEntitiesVisitor(newTestNormalizer(false)), `
			union Species = Cat | Dog

			type Mammal @key(fields: "species") {
				species: Species!
			}
		`, invalidPrimaryKeyTypeErrorMessage("Mammal", "species"))
	})
}

var collectEntities = func(t *testing.T, visitor *collectValidEntitiesVisitor, operation string, expectedEntities entitySet) {
	operationDocument := unsafeparser.ParseGraphqlDocumentString(operation)
	report := operationreport.Report{}
	walker := astvisitor.NewWalker(48)

	visitor.Register(&walker)

	walker.Walk(&operationDocument, nil, &report)

	if report.HasErrors() {
		t.Fatal(report.Error())
	}

	got := visitor.normalizer.entityValidator.entitySet

	assert.Equal(t, expectedEntities, got)
}

var collectEntitiesAndExpectError = func(t *testing.T, visitor *collectValidEntitiesVisitor, operation string, expectedError string) {
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

func invalidPrimaryKeyTypeErrorMessage(typeName, primaryKey string) string {
	return fmt.Sprintf("an extension of the entity named '%s' has a field named '%s' whose type is invalid (union or interface) for a primary key", typeName, primaryKey)
}
