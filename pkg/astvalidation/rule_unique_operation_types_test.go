package astvalidation

import (
	"testing"
)

func TestUniqueOperationTypes(t *testing.T) {
	t.Run("Definition", func(t *testing.T) {
		t.Run("no schema definition", func(t *testing.T) {
			runDefinitionValidation(t, `
					type Foo
				`, Valid, UniqueOperationTypes(),
			)
		})

		t.Run("schema definition with all types", func(t *testing.T) {
			runDefinitionValidation(t, `
					type Foo
					schema {
						query: Foo
						mutation: Foo
						subscription: Foo
					}
				`, Valid, UniqueOperationTypes(),
			)
		})

		t.Run("schema definition with single extension", func(t *testing.T) {
			runDefinitionValidation(t, `
					type Foo
					schema { query: Foo }
					extend schema {
						mutation: Foo
						subscription: Foo
					}
				`, Valid, UniqueOperationTypes(),
			)
		})

		t.Run("schema definition with separate extensions", func(t *testing.T) {
			runDefinitionValidation(t, `
					type Foo
					schema { query: Foo }
					extend schema { mutation: Foo }
					extend schema { subscription: Foo }
				`, Valid, UniqueOperationTypes(),
			)
		})

		t.Run("duplicate operation types inside single schema definition", func(t *testing.T) {
			runDefinitionValidation(t, `
					type Foo
					schema {
						query: Foo
						mutation: Foo
						subscription: Foo
						query: Foo
						mutation: Foo
						subscription: Foo
					}
				`, Invalid, UniqueOperationTypes(),
			)
		})

		t.Run("duplicate operation types inside schema extension", func(t *testing.T) {
			runDefinitionValidation(t, `
					type Foo
					schema {
						query: Foo
						mutation: Foo
						subscription: Foo
					}
					extend schema {
						query: Foo
						mutation: Foo
						subscription: Foo
					}
				`, Invalid, UniqueOperationTypes(),
			)
		})

		t.Run("duplicate operation types inside schema extension twice", func(t *testing.T) {
			runDefinitionValidation(t, `
					type Foo
					schema {
						query: Foo
						mutation: Foo
						subscription: Foo
					}
					extend schema {
						query: Foo
						mutation: Foo
						subscription: Foo
					}
					extend schema {
						query: Foo
						mutation: Foo
						subscription: Foo
					}
				`, Invalid, UniqueOperationTypes(),
			)
		})

		t.Run("duplicate operation types inside second schema extension", func(t *testing.T) {
			runDefinitionValidation(t, `
					type Foo
					schema {
						query: Foo
					}
					extend schema {
						mutation: Foo
						subscription: Foo
					}
					extend schema {
						query: Foo
						mutation: Foo
						subscription: Foo
					}
				`, Invalid, UniqueOperationTypes(),
			)
		})
	})
}
