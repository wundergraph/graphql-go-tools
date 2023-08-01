package sdlmerge

import (
	"testing"
)

func TestRemoveFieldDefinitionDirective(t *testing.T) {
	t.Run("remove specified directive from field definition", func(t *testing.T) {
		run(
			t,
			newRemoveFieldDefinitionDirective("requires", "provides"),
			`
				type Dog {
		          name: String!
                  age: Int!
                  code: String @requires(fields: "name age")
                  owner: Owner @provides(fields: "name")
				}
			`,
			`
				type Dog {
		          name: String!
                  age: Int!
                  code: String
                  owner: Owner
				}`,
		)
	})
}
