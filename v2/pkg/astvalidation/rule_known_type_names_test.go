package astvalidation

import (
	"testing"
)

func TestKnownTypeNames(t *testing.T) {
	t.Run("Definition", func(t *testing.T) {
		t.Run("use standard scalars", func(t *testing.T) {
			runDefinitionValidation(t, `
					type Query {
						string: String
						int: Int
						float: Float
						boolean: Boolean
						id: ID
					}
				`, Valid, KnownTypeNames(),
			)
		})

		t.Run("reference types defined inside the same document", func(t *testing.T) {
			runDefinitionValidation(t, `
					union SomeUnion = SomeObject | AnotherObject
					type SomeObject implements SomeInterface {
						someScalar(arg: SomeInputObject): SomeScalar
					}
					type AnotherObject {
						foo(arg: SomeInputObject): String
					}
					type SomeInterface {
						someScalar(arg: SomeInputObject): SomeScalar
					}
					input SomeInputObject {
						someScalar: SomeScalar
					}
					scalar SomeScalar
					type RootQuery {
						someInterface: SomeInterface
						someUnion: SomeUnion
						someScalar: SomeScalar
						someObject: SomeObject
					}
					schema {
						query: RootQuery
					}
				`, Valid, KnownTypeNames(),
			)
		})

		t.Run("reference types defined inside extension", func(t *testing.T) {
			runDefinitionValidation(t, `
					type Foo
					type Bar
					type Query {
						foo: Foo
					}

					extend type Query {
						bar: Bar
					}
				`, Valid, KnownTypeNames(),
			)
		})

		t.Run("unknown type references", func(t *testing.T) {
			runDefinitionValidation(t, `
					type A
					type B
					type SomeObject implements C {
						e(d: D): E
					}
					union SomeUnion = F | G
					interface SomeInterface {
						i(h: H): I
					}
					input SomeInput {
						j: J
					}
					directive @SomeDirective(k: K) on QUERY
					schema {
						query: L
						mutation: M
						subscription: N
					}
				`, Invalid, KnownTypeNames(),
			)
		})

		t.Run("unknown type references: root operation", func(t *testing.T) {
			runDefinitionValidation(t, `
					type Foo
					schema {
						query: Query
						mutation: B
					}

					type Query {
						foo: Foo
					}
				`, Invalid, KnownTypeNames(),
			)
		})

		t.Run("unknown type references: interface", func(t *testing.T) {
			runDefinitionValidation(t, `
					interface Foo {
						bar: Bar
					}

					type Query {
						foo: Foo
					}
				`, Invalid, KnownTypeNames(),
			)
		})

		t.Run("unknown type references: union", func(t *testing.T) {
			runDefinitionValidation(t, `
					union Foo = Bar | Baz

					type Query {
						foo: Foo
					}
				`, Invalid, KnownTypeNames(),
			)
		})

		t.Run("unknown type references: input", func(t *testing.T) {
			runDefinitionValidation(t, `
					input Foo {
						bar: Bar
					}

					type Query {
						foo: String
					}
				`, Invalid, KnownTypeNames(),
			)
		})

		t.Run("unknown type references: argument", func(t *testing.T) {
			runDefinitionValidation(t, `
					type Query {
						foo(bar: Bar): String
					}
				`, Invalid, KnownTypeNames(),
			)
		})

		t.Run("unknown type references: directive", func(t *testing.T) {
			runDefinitionValidation(t, `
					directive @SomeDirective(bar: Baz) on QUERY
					schema {
						query: MyQuery
					}

					type MyQuery {
						foo: String
					}
				`, Invalid, KnownTypeNames(),
			)
		})

		t.Run("does not consider non-type definitions", func(t *testing.T) {
			runDefinitionValidation(t, `
					fragment Foo on Query { __typename }
					directive @Foo on QUERY
					type Query {
						foo: Foo
						__typename: String!
					}
				`, Invalid, KnownTypeNames(),
			)
		})

		t.Run("unkown reference type defined inside extension", func(t *testing.T) {
			runDefinitionValidation(t, `
					type Foo
					type Query {
						foo: Foo
					}

					extend type Query {
						bar: Bar
					}
				`, Invalid, KnownTypeNames(),
			)
		})
	})
}
