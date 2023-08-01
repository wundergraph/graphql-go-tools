package sdlmerge

import (
	"testing"
)

func TestRemoveFieldDirective(t *testing.T) {
	t.Run("remove field with specified directive", func(t *testing.T) {
		run(
			t, newRemoveFieldDefinitions("forDelete"),
			`
				type Dog {
					name: String @notForDelete
					favoriteToy: String @forDelete
					barkVolume: Int
					isHousetrained(atOtherHomes: Boolean): Boolean! @forDelete
					doesKnowCommand(dogCommand: DogCommand!): Boolean!
				}
			`,
			`
				type Dog {
					name: String @notForDelete	
					barkVolume: Int
					doesKnowCommand(dogCommand: DogCommand!): Boolean!
				}
			`)
	})
}
