package astvalidation

import (
	"testing"
)

func TestPopulatedTypeBodies(t *testing.T) {
	t.Run("Definition", func(t *testing.T) {
		t.Run("Populated type bodies are valid", func(t *testing.T) {
			runDefinitionValidation(t, `
					enum Species {
						CAT
					}

					extend enum Color {
						DOG
					}

					input Message {
						content: String!
					}

					extend input Message {
						updated: DateTime!
					}
					
					interface Animal {
						species: Species!
					}
					
					extend interface Animal {
						age: Int!
					}
					
					type Cat implements Animal {
						species: Species!
					}
					
					extend type Cat implements Animal {
						age: Int!
					}
				`, Valid, PopulatedTypeBodies(),
			)
		})

		t.Run("Empty enum is invalid", func(t *testing.T) {
			runDefinitionValidation(t, `
					enum Species {
					}
				`, Invalid, PopulatedTypeBodies(),
			)
		})

		t.Run("Empty enum extension is invalid", func(t *testing.T) {
			runDefinitionValidation(t, `
					enum Species {
						CAT
					}

					extend enum Species {
					}
				`, Invalid, PopulatedTypeBodies(),
			)
		})

		t.Run("Empty input is invalid", func(t *testing.T) {
			runDefinitionValidation(t, `
					input Message {
					}
				`, Invalid, PopulatedTypeBodies(),
			)
		})

		t.Run("Empty input extension is invalid", func(t *testing.T) {
			runDefinitionValidation(t, `
					input Message {
						content: String!
					}

					extend input Message {
					}
				`, Invalid, PopulatedTypeBodies(),
			)
		})

		t.Run("Empty interface is invalid", func(t *testing.T) {
			runDefinitionValidation(t, `
					interface Animal {
					}
				`, Invalid, PopulatedTypeBodies(),
			)
		})

		t.Run("Empty interface extension is invalid", func(t *testing.T) {
			runDefinitionValidation(t, `
					interface Animal {
						species: String!
					}

					extend interface Animal {
					}
				`, Invalid, PopulatedTypeBodies(),
			)
		})

		t.Run("Empty object is invalid", func(t *testing.T) {
			runDefinitionValidation(t, `
					type Cat {
					}
				`, Invalid, PopulatedTypeBodies(),
			)
		})

		t.Run("Empty object extension is invalid", func(t *testing.T) {
			runDefinitionValidation(t, `
					type Cat {
						species: String!
					}

					extend type Cat {
					}
				`, Invalid, PopulatedTypeBodies(),
			)
		})
	})
}
