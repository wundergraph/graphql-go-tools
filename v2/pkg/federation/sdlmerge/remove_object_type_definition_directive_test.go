package sdlmerge

import (
	"testing"
)

func TestRemoveObjectTypeDefinitionDirective(t *testing.T) {
	t.Run("remove specified directive from object definition", func(t *testing.T) {
		run(
			t, newRemoveObjectTypeDefinitionDirective("key"),
			`
			type Dog @key(fields: "name") {
				name: String
				favoriteToy: String
				barkVolume: Int
			}
			`,
			`
			type Dog {
				name: String
				favoriteToy: String
				barkVolume: Int
			}
			`)
	})
	t.Run("remove multiple specified directive from object definition", func(t *testing.T) {
		run(
			t, newRemoveObjectTypeDefinitionDirective("key"),
			`
			type Dog @key(fields: "name") @key(fields: "favoriteToy") {
				name: String
				favoriteToy: String
				barkVolume: Int
			}
			`,
			`
			type Dog {
				name: String
				favoriteToy: String
				barkVolume: Int
			}
			`)
	})
	t.Run("remove specified directive from object definition with different directives", func(t *testing.T) {
		run(
			t, newRemoveObjectTypeDefinitionDirective("key"),
			`
			type Dog @key(fields: "name") @notForDeletion(fields: "favoriteToy") {
				name: String
				favoriteToy: String
				barkVolume: Int
			}
			`,
			`
			type Dog @notForDeletion(fields: "favoriteToy") {
				name: String
				favoriteToy: String
				barkVolume: Int
			}
			`)
	})
}
