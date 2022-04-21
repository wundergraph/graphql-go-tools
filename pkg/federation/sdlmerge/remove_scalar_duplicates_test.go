package sdlmerge

import "testing"

func TestRemoveScalarDuplicates(t *testing.T) {
	t.Run("Input and output are identical when no duplications", func(t *testing.T) {
		run(t, newRemoveDuplicateScalarTypeDefinitionVisitor(), `
					scalar DateTime
					 `, `
					scalar DateTime
					`)
	})

	t.Run("Same named scalars are removed to leave only one", func(t *testing.T) {
		run(t, newRemoveDuplicateScalarTypeDefinitionVisitor(), `
					scalar DateTime
	
					scalar DateTime

					scalar DateTime
					 `, `
					scalar DateTime
					`)
	})

	t.Run("Any more than one of a same named scalar are removed", func(t *testing.T) {
		run(t, newRemoveDuplicateScalarTypeDefinitionVisitor(), `
					scalar DateTime

					scalar BigInt
	
					scalar BigInt
					
					scalar CustomScalar

					scalar DateTime
	
					scalar UniqueScalar
	
					scalar BigInt
					
					scalar CustomScalar
					
					scalar CustomScalar
	
					scalar DateTime

					scalar CustomScalar

					scalar DateTime
					 `, `
					scalar DateTime

					scalar BigInt

					scalar CustomScalar

					scalar UniqueScalar
					`)
	})
}
