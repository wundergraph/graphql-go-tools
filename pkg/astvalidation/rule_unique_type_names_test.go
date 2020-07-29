package astvalidation

import (
	"testing"
)

func TestUniqueTypeNames(t *testing.T) {
	t.Run("Definition", func(t *testing.T) {
		t.Run("no types", func(t *testing.T) {
			runDefinitionValidation(t, `
					directive @test on SCHEMA
				`, Valid, UniqueTypeNames(),
			)
		})

		t.Run("one type", func(t *testing.T) {
			runDefinitionValidation(t, `
					type Foo
				`, Valid, UniqueTypeNames(),
			)
		})

		t.Run("many types", func(t *testing.T) {
			runDefinitionValidation(t, `
					type Foo
					type Bar
					type Baz
				`, Valid, UniqueTypeNames(),
			)
		})

		t.Run("type and non-type definitions named the same", func(t *testing.T) {
			// Note: graphql-js uses following test case
			//   query Foo { __typename }
			//   fragment Foo on Query { __typename }
			//   directive @Foo on SCHEMA
			//   type Foo
			runDefinitionValidation(t, `
					directive @Foo on SCHEMA
					type Foo
				`, Valid, UniqueTypeNames(),
			)
		})

		t.Run("types and non-types named different", func(t *testing.T) {
			runDefinitionValidation(t, `
					type Foo
					scalar FooScalar
					type FooBar
					interface FooInterface
					union FooUnion
					enum FooEnum
					input FooInput
				`, Valid, UniqueTypeNames(),
			)
		})

		t.Run("types named the same", func(t *testing.T) {
			runDefinitionValidation(t, `
					type Foo
					type Foo
				`, Invalid, UniqueTypeNames(),
			)
		})

		t.Run("type and scalar named the same", func(t *testing.T) {
			runDefinitionValidation(t, `
					type Foo
					scalar Foo
				`, Invalid, UniqueTypeNames(),
			)
		})

		t.Run("type and interface named the same", func(t *testing.T) {
			runDefinitionValidation(t, `
					type Foo
					interface Foo
				`, Invalid, UniqueTypeNames(),
			)
		})

		t.Run("type and union named the same", func(t *testing.T) {
			runDefinitionValidation(t, `
					type Foo
					union Foo
				`, Invalid, UniqueTypeNames(),
			)
		})

		t.Run("type and enum named the same", func(t *testing.T) {
			runDefinitionValidation(t, `
					type Foo
					enum Foo
				`, Invalid, UniqueTypeNames(),
			)
		})

		t.Run("type and input named the same", func(t *testing.T) {
			runDefinitionValidation(t, `
					type Foo
					input Foo
				`, Invalid, UniqueTypeNames(),
			)
		})
	})
}
