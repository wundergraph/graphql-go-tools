package astnormalization

import "testing"

func TestRemoveTypeExtensions(t *testing.T) {
	t.Run("remove single type extension of fieldDefinition", func(t *testing.T) {
		runManyOnDefinition(t, `
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
					`,
			extendObjectTypeDefinition,
			removeMergedTypeExtensions)
	})
	t.Run("remove single type extension of directive", func(t *testing.T) {
		runManyOnDefinition(t, `
					type Cat {
						name: String
					}
					extend type Cat @deprecated(reason: "not as cool as dogs")
					 `, `
					type Cat @deprecated(reason: "not as cool as dogs") {
						name: String
					}
					`,
			extendObjectTypeDefinition,
			removeMergedTypeExtensions)
	})
	t.Run("remove multiple type extensions at once", func(t *testing.T) {
		runManyOnDefinition(t, `
					type Cat {
						name: String
					}
					extend type Cat @deprecated(reason: "not as cool as dogs")
					extend type Cat {
						age: Int
					}
					 `, `
					type Cat @deprecated(reason: "not as cool as dogs") {
						name: String
						age: Int
					}
					`,
			extendObjectTypeDefinition,
			removeMergedTypeExtensions)
	})
	t.Run("remove scalar type extensions", func(t *testing.T) {
		runManyOnDefinition(t, `
					scalar Coordinates
					extend scalar Coordinates @deprecated(reason: "some reason") @skip(if: false)
					 `, `
					scalar Coordinates @deprecated(reason: "some reason") @skip(if: false)
					`,
			extendScalarTypeDefinition,
			removeMergedTypeExtensions)
	})
	t.Run("remove enum type extensions", func(t *testing.T) {
		runManyOnDefinition(t, `
					enum Countries {DE ES NL}
					extend enum Countries @deprecated(reason: "some reason") @skip(if: false) {EN IT}
					 `, `
					enum Countries @deprecated(reason: "some reason") @skip(if: false) {DE ES NL EN IT}
					`,
			extendEnumTypeDefinition,
			removeMergedTypeExtensions)
	})
	t.Run("remove union type extensions", func(t *testing.T) {
		runManyOnDefinition(t, `
					union Mammal
					extend union Mammal @deprecated(reason: "some reason") @skip(if: false) = Cat | Dog
					 `, `
					union Mammal @deprecated(reason: "some reason") @skip(if: false) = Cat | Dog
					`,
			extendUnionTypeDefinition,
			removeMergedTypeExtensions)
	})
	t.Run("remove input object type extensions", func(t *testing.T) {
		runManyOnDefinition(t, `
					input DogSize {width: Float height: Float}
					extend input DogSize @deprecated(reason: "some reason") @skip(if: false) {breadth: Float weight: Float}
					 `, `
					input DogSize @deprecated(reason: "some reason") @skip(if: false) {width: Float height: Float breadth: Float weight: Float}
					`,
			extendInputObjectTypeDefinition,
			removeMergedTypeExtensions)
	})
	t.Run("remove interface type extensions", func(t *testing.T) {
		runManyOnDefinition(t, `
					interface Mammal {
						name: String
					}
					extend interface Mammal @deprecated(reason: "some reason") @skip(if: false) {
						furType: String
						age: Int
					}
					 `, `
					interface Mammal @deprecated(reason: "some reason") @skip(if: false) {
						name: String
						furType: String
						age: Int
					}
					`,
			extendInterfaceTypeDefinition,
			removeMergedTypeExtensions)
	})

	t.Run("remove object type extensions when object type definition does not exist", func(t *testing.T) {
		runManyOnDefinition(t, `
					extend type Query {
  						_entities(representations: [_Any!]!): [_Entity]!
  						_service: _Service!
					}

					extend type Query {
						me: User
					}
					`, `
					type Query {
  						_entities(representations: [_Any!]!): [_Entity]!
  						_service: _Service!
						me: User
					}
					`,
			extendObjectTypeDefinition,
			removeMergedTypeExtensions)
	})

	t.Run("remove input object type extensions when input object type definition does not exist", func(t *testing.T) {
		runManyOnDefinition(t, `
					extend input Location {
  						lat: Float
					}

					extend input Location {
						lon: Float
					}
					`, `
					input Location {
  						lat: Float
						lon: Float
					}
					`,
			extendInputObjectTypeDefinition,
			removeMergedTypeExtensions)
	})

	t.Run("remove enum type extensions when enum type does not exist", func(t *testing.T) {
		runManyOnDefinition(t, `
					extend enum Planet {
  						EARTH
					}

					extend enum Planet {
						MARS
					}
					`, `
					enum Planet {
  						EARTH
						MARS
					}
					`,
			extendEnumTypeDefinition,
			removeMergedTypeExtensions)
	})

	t.Run("remove interface type extensions when interface type does not exist", func(t *testing.T) {
		runManyOnDefinition(t, `
					extend interface Entity {
  						id: ID
					}

					extend interface Entity {
						createdAt: String
					}
					`, `
					interface Entity {
  						id: ID
						createdAt: String
					}
					`,
			extendInterfaceTypeDefinition,
			removeMergedTypeExtensions)
	})

	t.Run("remove scalar type extensions when scalar type does not exist", func(t *testing.T) {
		runManyOnDefinition(t, `
					extend scalar IPv4
					extend scalar IPv4 @deprecated(reason: "use IPv6")
					`, `
					scalar IPv4 @deprecated(reason: "use IPv6")
					`,
			extendScalarTypeDefinition,
			removeMergedTypeExtensions)
	})

	t.Run("remove union type extensions when union type does not exist", func(t *testing.T) {
		runManyOnDefinition(t, `
					extend union Response = SuccessResponse
					extend union Response = ErrorResponse
					`, `
					union Response = SuccessResponse | ErrorResponse
					`,
			extendUnionTypeDefinition,
			removeMergedTypeExtensions)
	})
}
