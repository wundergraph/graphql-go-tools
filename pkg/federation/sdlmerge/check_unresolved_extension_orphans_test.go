package sdlmerge

import (
	"fmt"
	"testing"
)

func TestCheckUnresolvedExtensionOrphans(t *testing.T) {
	t.Run("Unresolved enum extension orphan returns an error", func(t *testing.T) {
		runAndExpectError(t, newCheckUnresolvedExtensionOrphansVisitor(), `
			extend enum Badges {
				BOULDER
			}
		`, UnresolvedExtensionOrphansErrorMessage("Badges"))
	})

	t.Run("Unresolved input extension orphan returns an error", func(t *testing.T) {
		runAndExpectError(t, newCheckUnresolvedExtensionOrphansVisitor(), `
			extend input Badges {
				name: String!
			}
		`, UnresolvedExtensionOrphansErrorMessage("Badges"))
	})

	t.Run("Unresolved interface extension orphan returns an error", func(t *testing.T) {
		runAndExpectError(t, newCheckUnresolvedExtensionOrphansVisitor(), `
			extend interface Badges {
				name: String!
			}
		`, UnresolvedExtensionOrphansErrorMessage("Badges"))
	})

	t.Run("Unresolved object extension orphan returns an error", func(t *testing.T) {
		runAndExpectError(t, newCheckUnresolvedExtensionOrphansVisitor(), `
			extend type Badges {
				name: String!
			}
		`, UnresolvedExtensionOrphansErrorMessage("Badges"))
	})

	t.Run("Unresolved scalar extension orphan returns an error", func(t *testing.T) {
		runAndExpectError(t, newCheckUnresolvedExtensionOrphansVisitor(), `
			extend scalar Badges @onScalar
		`, UnresolvedExtensionOrphansErrorMessage("Badges"))
	})

	t.Run("Unresolved union extension orphan returns an error", func(t *testing.T) {
		runAndExpectError(t, newCheckUnresolvedExtensionOrphansVisitor(), `
			extend union Badges = Boulder
		`, UnresolvedExtensionOrphansErrorMessage("Badges"))
	})
}

func UnresolvedExtensionOrphansErrorMessage(typeName string) string {
	return fmt.Sprintf("the extension orphan named '%s' was never resolved in the supergraph", typeName)
}
