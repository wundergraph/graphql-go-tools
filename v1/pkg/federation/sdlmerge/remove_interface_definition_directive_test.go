package sdlmerge

import (
	"testing"
)

func TestRemoveInterfaceDefinitionDirective(t *testing.T) {
	t.Run("remove specified directive from interface", func(t *testing.T) {
		run(
			t, newRemoveInterfaceDefinitionDirective("key"),
			`
			interface Mammal @key(fields: "name")  {
					name: String
			}
			`,
			`
				interface Mammal {
					name: String
				}
			`)
	})
	t.Run("remove multiple specified directive from interface", func(t *testing.T) {
		run(
			t, newRemoveInterfaceDefinitionDirective("key"),
			`
			interface Mammal @key(fields: "name") @key(fields: "favoriteToy") {
					name: String
			}
			`,
			`
				interface Mammal {
					name: String
				}
			`)
	})
	t.Run("remove specified directive from interface with different directives", func(t *testing.T) {
		run(
			t, newRemoveInterfaceDefinitionDirective("key"),
			`
			interface Mammal @key(fields: "name") @notForDeletion(fields: "favoriteToy") {
					name: String
			}
			`,
			`
			interface Mammal @notForDeletion(fields: "favoriteToy") {
				name: String
			}
			`)
	})
}
