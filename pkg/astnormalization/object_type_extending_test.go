package astnormalization

import "testing"

func TestExtendObjectType(t *testing.T) {
	t.Run("extend object type by field", func(t *testing.T) {
		run(t, extendObjectTypeDefinition, testDefinition, `
					type Dog {
						name: String
					}
					extend type Dog {
						favoriteToy: String
					}
					 `, `
					type Dog {
						name: String
						favoriteToy: String
					}
					extend type Dog {
						favoriteToy: String
					}
					`)
	})
	t.Run("extend object type by directive", func(t *testing.T) {
		run(t, extendObjectTypeDefinition, testDefinition, `
					type Cat {
						name: String
					}
					extend type Cat @deprecated(reason: "not as cool as dogs")
					 `, `
					type Cat @deprecated(reason: "not as cool as dogs") {
						name: String
					}
					extend type Cat @deprecated(reason: "not as cool as dogs")
					`)
	})
	t.Run("extend object type by multiple field", func(t *testing.T) {
		run(t, extendObjectTypeDefinition, testDefinition, `
					type Dog {
						name: String
					}
					extend type Dog {
						favoriteToy: String
						breed: String
					}
					 `, `
					type Dog {
						name: String
						favoriteToy: String
						breed: String
					}
					extend type Dog {
						favoriteToy: String
						breed: String
					}
					`)
	})
	t.Run("extend object type by multiple directives", func(t *testing.T) {
		run(t, extendObjectTypeDefinition, testDefinition, `
					type Cat {
						name: String
					}
					extend type Cat @deprecated(reason: "not as cool as dogs") @skip(if: false)
					 `, `
					type Cat @deprecated(reason: "not as cool as dogs") @skip(if: false) {
						name: String
					}
					extend type Cat @deprecated(reason: "not as cool as dogs") @skip(if: false)
					`)
	})
	t.Run("extend object type by complex extends", func(t *testing.T) {
		run(t, extendObjectTypeDefinition, testDefinition, `
					type Cat {
						name: String
					}
					extend type Cat @deprecated(reason: "not as cool as dogs") @skip(if: false) {
						age: Int
						breed: String
					}
					 `, `
					type Cat @deprecated(reason: "not as cool as dogs") @skip(if: false) {
						name: String
						age: Int
						breed: String
					}
					extend type Cat @deprecated(reason: "not as cool as dogs") @skip(if: false) {
						age: Int
						breed: String
					}
					`)
	})
	t.Run("extend missing object type definition", func(t *testing.T) {
		run(t, extendObjectTypeDefinition, `schema { query: Query }`, `
			extend type Query {	me: String }
			extend type Cat @deprecated(reason: "not as cool as dogs") @skip(if: false) { age: Int breed: String }
			`, `
			extend type Query { me: String }
			extend type Cat @deprecated(reason: "not as cool as dogs") @skip(if: false) { age: Int breed: String }
			type Query { me: String	}
			type Cat @deprecated(reason: "not as cool as dogs") @skip(if: false) { age: Int breed: String }
			`)
	})
	t.Run("extend object type by interface", func(t *testing.T) {
		run(t, extendObjectTypeDefinition, testDefinition, `
					type Dog {
						name: String
					}
					extend type Dog implements ToyLover {
						favoriteToy: String
					}
					interface ToyLover {
						favoriteToy: String
					}
					 `, `
					type Dog implements ToyLover {
						name: String
						favoriteToy: String
					}
					extend type Dog implements ToyLover {
						favoriteToy: String
					}
					interface ToyLover {
						favoriteToy: String
					}
					`)
	})
	t.Run("extend object type which implements interface by interface", func(t *testing.T) {
		run(t, extendObjectTypeDefinition, testDefinition, `
					type Dog implements ToyHater {
						name: String
						hatedToy: String
					}
					extend type Dog implements ToyLover {
						favoriteToy: String
					}
					interface ToyLover {
						favoriteToy: String
					}
					interface ToyHater {
						hatedToy: String
					}
					 `, `
					type Dog implements ToyHater & ToyLover {
						name: String
						hatedToy: String
						favoriteToy: String
					}
					extend type Dog implements ToyLover {
						favoriteToy: String
					}
					interface ToyLover {
						favoriteToy: String
					}
					interface ToyHater {
						hatedToy: String
					}
					`)
	})
}
