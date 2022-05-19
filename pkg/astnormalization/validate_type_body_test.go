package astnormalization

import (
	"fmt"
	"testing"
)

func TestCheckUnresolvedExtensionOrphans(t *testing.T) {
	t.Run("Empty enum body returns an error", func(t *testing.T) {
		runAndExpectError(t, validateTypeBodyVisitor, testDefinition, `
			enum Badges {
			}
		`, EmptyTypeBodyErrorMessage("enum", "Badges"))
	})

	t.Run("Empty input body returns an error", func(t *testing.T) {
		runAndExpectError(t, validateTypeBodyVisitor, testDefinition, `
			input Badges {
			}
		`, EmptyTypeBodyErrorMessage("input", "Badges"))
	})

	t.Run("Empty interface body returns an error", func(t *testing.T) {
		runAndExpectError(t, validateTypeBodyVisitor, testDefinition, `
			interface Badges {
			}
		`, EmptyTypeBodyErrorMessage("interface", "Badges"))
	})

	t.Run("Empty object body returns an error", func(t *testing.T) {
		runAndExpectError(t, validateTypeBodyVisitor, testDefinition, `
			type Badges {
			}
		`, EmptyTypeBodyErrorMessage("object", "Badges"))
	})
}

func EmptyTypeBodyErrorMessage(definitionType, typeName string) string {
	return fmt.Sprintf("the %s named '%s' is invalid due to an empty body", definitionType, typeName)
}
