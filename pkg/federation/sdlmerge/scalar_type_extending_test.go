package sdlmerge

import "testing"

func TestExtendScalarType(t *testing.T) {
	t.Run("Scalar types can be extended", func(t *testing.T) {
		run(t, newExtendScalarTypeDefinition(), `
			scalar Attack
			extend scalar Attack @deprecated(reason: "some reason") @skip(if: false)
		`, `
			scalar Attack @deprecated(reason: "some reason") @skip(if: false)
			extend scalar Attack @deprecated(reason: "some reason") @skip(if: false)
		`)
	})

	t.Run("Extending a scalar that is a shared type returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendScalarTypeDefinition(), `
			scalar Attack
			scalar Attack
			extend scalar Attack @deprecated(reason: "some reason") @skip(if: false)
		`, sharedTypeExtensionErrorMessage("Attack"))
	})

	t.Run("Unresolved scalar extension orphan returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendScalarTypeDefinition(), `
			extend scalar Badges @onScalar
		`, unresolvedExtensionOrphansErrorMessage("Badges"))
	})
}
