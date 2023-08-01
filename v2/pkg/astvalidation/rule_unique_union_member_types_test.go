package astvalidation

import (
	"testing"
)

func TestUniqueMemberTypes(t *testing.T) {
	t.Run("Definition", func(t *testing.T) {
		t.Run("Union with a single member is valid", func(t *testing.T) {
			runDefinitionValidation(t, `
					union Foo = Bar
				`, Valid, UniqueUnionMemberTypes(),
			)
		})

		t.Run("Union with many members is valid", func(t *testing.T) {
			runDefinitionValidation(t, `
					union Foo = Bar | FooBar | BarFoo
				`, Valid, UniqueUnionMemberTypes(),
			)
		})

		t.Run("Union with duplicate members is invalid", func(t *testing.T) {
			runDefinitionValidation(t, `
					union Foo = Bar | Bar
				`, Invalid, UniqueUnionMemberTypes(),
			)
		})
	})
}
