package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCollectValidEntities(t *testing.T) {
	t.Run("Valid entities are collected", func(t *testing.T) {
		collectEntities(t, newCollectValidEntitiesVisitor(&normalizer{}), `
			type Dog @key(fields: "name") @key(fields: "id"){
				id: ID!
				name: String!
			}
		`, map[string]map[string]bool{"Dog": {"name": true, "id": true}})
	})
}

var collectEntities = func(t *testing.T, visitor *collectValidEntitiesVisitor, operation string, expectedEntities map[string]map[string]bool) {
	operationDocument := unsafeparser.ParseGraphqlDocumentString(operation)
	report := operationreport.Report{}
	walker := astvisitor.NewWalker(48)

	visitor.Register(&walker)

	walker.Walk(&operationDocument, nil, &report)

	if report.HasErrors() {
		t.Fatal(report.Error())
	}

	got := visitor.normalizer.entities

	assert.Equal(t, expectedEntities, got)
}
