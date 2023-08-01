package sdlmerge

import (
	"testing"
)

func TestRemoveEmptyObjectTypeDefinitionDirective(t *testing.T) {
	t.Run("remove object definition without fields", func(t *testing.T) {
		run(t, newRemoveEmptyObjectTypeDefinition(), `
			type Query { 
			} 
			type Cat {
				name: String!
			}
		`, `
			type Cat {
				name: String!
			}
		`)
	})
}
