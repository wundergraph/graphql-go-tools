package astvalidation

import (
	"testing"
)

func TestUniqueEnumValueNames(t *testing.T) {
	t.Run("Definition", func(t *testing.T) {
		t.Run("no values", func(t *testing.T) {
			runDefinitionValidation(t, `
					enum SomeEnum
				`, Valid, UniqueEnumValueNames(),
			)
		})

		t.Run("one value", func(t *testing.T) {
			runDefinitionValidation(t, `
					enum SomeEnum {
						FOO
					}
				`, Valid, UniqueEnumValueNames(),
			)
		})

		t.Run("multiple values", func(t *testing.T) {
			runDefinitionValidation(t, `
					enum SomeEnum {
						FOO
						BAR
					}
				`, Valid, UniqueEnumValueNames(),
			)
		})

		t.Run("extend enum with new value", func(t *testing.T) {
			runDefinitionValidation(t, `
					enum SomeEnum {
						FOO
					}
					extend enum SomeEnum {
						BAR
					}
					extend enum SomeEnum {
						BAZ
					}
				`, Valid, UniqueEnumValueNames(),
			)
		})

		t.Run("duplicate values inside the same enum definition", func(t *testing.T) {
			runDefinitionValidation(t, `
					enum SomeEnum {
						FOO
						BAR
						FOO
					}
				`, Invalid, UniqueEnumValueNames(),
			)
		})

		t.Run("extend enum with duplicate value", func(t *testing.T) {
			runDefinitionValidation(t, `
					extend enum SomeEnum {
						FOO
					}
					enum SomeEnum {
						FOO
					}
				`, Invalid, UniqueEnumValueNames(),
			)
		})

		t.Run("duplicate value inside extension", func(t *testing.T) {
			runDefinitionValidation(t, `
					enum SomeEnum
					extend enum SomeEnum {
						FOO
						BAR
						FOO
					}
				`, Invalid, UniqueEnumValueNames(),
			)
		})

		t.Run("duplicate value inside different extensions", func(t *testing.T) {
			runDefinitionValidation(t, `
					enum SomeEnum
					extend enum SomeEnum {
						FOO
					}
					extend enum SomeEnum {
						FOO
					}
				`, Invalid, UniqueEnumValueNames(),
			)
		})
	})
}
