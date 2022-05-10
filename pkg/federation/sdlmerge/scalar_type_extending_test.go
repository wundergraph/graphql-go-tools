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

	// When federating, duplicate value types must be identical or the federation will fail.
	// Consequently, when extending, all duplicate value types should also be extended.
	t.Run("Duplicate unions are each extended", func(t *testing.T) {
		run(t, newExtendScalarTypeDefinition(), `
			scalar Attack
			scalar Attack
			extend scalar Attack @deprecated(reason: "some reason") @skip(if: false)
		`, `
			scalar Attack @deprecated(reason: "some reason") @skip(if: false)
			scalar Attack @deprecated(reason: "some reason") @skip(if: false)
			extend scalar Attack @deprecated(reason: "some reason") @skip(if: false)
		`)
	})
}
