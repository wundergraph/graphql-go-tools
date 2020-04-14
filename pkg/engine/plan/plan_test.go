package plan

import (
	"testing"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/jensneuse/graphql-go-tools/pkg/asttransform"
)

func TestPlanner_Plan(t *testing.T) {
	test := func(definition string) func(t *testing.T) {
		return func(t *testing.T) {
			def := unsafeparser.ParseGraphqlDocumentString(definition)
			err := asttransform.MergeDefinitionWithBaseSchema(&def)
			if err != nil {
				t.Fatal(err)
			}
			NewPlanner(&def)
		}
	}

	t.Run("empty plan", test(testDefinition))
}

const testDefinition = ``