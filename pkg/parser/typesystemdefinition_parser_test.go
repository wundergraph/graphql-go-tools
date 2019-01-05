package parser

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"reflect"
	"testing"
)

func TestParser_parseTypeSystemDefinition(t *testing.T) {

	type checkFunc func(definition document.TypeSystemDefinition, parsedDefinitions ParsedDefinitions, err error)
	checks := func(checkFuncs ...checkFunc) []checkFunc { return checkFuncs }
	type Case struct {
		input  string
		checks []checkFunc
	}

	run := func(t *testing.T, c Case) {
		parser := NewParser()
		parser.l.SetInput(c.input)
		definition, err := parser.parseTypeSystemDefinition()
		for _, check := range c.checks {
			check(definition, parser.ParsedDefinitions, err)
		}
	}

	hasError := func() checkFunc {
		return func(definition document.TypeSystemDefinition, parsedDefinitions ParsedDefinitions, err error) {
			if err == nil {
				panic(fmt.Errorf("hasError: expected error, got nil"))
			}
		}
	}

	hasNoError := func() checkFunc {
		return func(definition document.TypeSystemDefinition, parsedDefinitions ParsedDefinitions, err error) {
			if err != nil {
				panic(err)
			}
		}
	}

	unionsParsed := func(n int) checkFunc {
		return func(definition document.TypeSystemDefinition, parsedDefinitions ParsedDefinitions, err error) {
			if len(definition.UnionTypeDefinitions) != n {
				panic(fmt.Errorf("unionsParsed: want (index): %d got: %d", n, len(definition.UnionTypeDefinitions)))
			}

			if len(parsedDefinitions.UnionTypeDefinitions) != n {
				panic(fmt.Errorf("unionsParsed: want (definition): %d got: %d", n, len(definition.UnionTypeDefinitions)))
			}
		}
	}

	hasUnion := func(name, description string, members ...string) checkFunc {
		return func(definition document.TypeSystemDefinition, parsedDefinitions ParsedDefinitions, err error) {
			for _, i := range definition.UnionTypeDefinitions {
				if name == parsedDefinitions.UnionTypeDefinitions[i].Name {

					if description != parsedDefinitions.UnionTypeDefinitions[i].Description {
						panic(fmt.Errorf("hasUnion: want description: %s got: %s", description, parsedDefinitions.UnionTypeDefinitions[i].Description))
					}

					if !reflect.DeepEqual(parsedDefinitions.UnionTypeDefinitions[i].UnionMemberTypes, document.UnionMemberTypes(members)) {
						panic(fmt.Errorf("hasUnion: want members: %+v got members: %+v", members, parsedDefinitions.UnionTypeDefinitions[i].UnionMemberTypes))
					}

					return
				}
			}

			panic(fmt.Errorf("hasUnion: want: %s (not found)", name))
		}
	}

	unionHasDirective := func(union, directive string) checkFunc {
		return func(definition document.TypeSystemDefinition, parsedDefinitions ParsedDefinitions, err error) {
			for _, i := range definition.UnionTypeDefinitions {
				u := parsedDefinitions.UnionTypeDefinitions[i]
				if u.Name == union {
					for _, k := range u.Directives {
						if parsedDefinitions.Directives[k].Name == directive {
							return
						}
					}
				}
			}

			panic(fmt.Errorf("unionHasDirective: not found for union: %s, directive: %s", union, directive))
		}
	}

	hasSchema := func(query, mutation, subscription string) checkFunc {
		return func(definition document.TypeSystemDefinition, parsedDefinitions ParsedDefinitions, err error) {
			if definition.SchemaDefinition.Query != query {
				panic(fmt.Errorf("hasSchema: want (Query): %s got: %s", query, definition.SchemaDefinition.Query))
			}
			if definition.SchemaDefinition.Mutation != mutation {
				panic(fmt.Errorf("hasSchema: want (Mutation): %s got: %s", mutation, definition.SchemaDefinition.Mutation))
			}
			if definition.SchemaDefinition.Subscription != subscription {
				panic(fmt.Errorf("hasSchema: want (Subscription): %s got: %s", subscription, definition.SchemaDefinition.Subscription))
			}
		}
	}

	hasScalar := func(name, description string, directives ...string) checkFunc {
		return func(definition document.TypeSystemDefinition, parsedDefinitions ParsedDefinitions, err error) {
			for _, i := range definition.ScalarTypeDefinitions {
				scalar := parsedDefinitions.ScalarTypeDefinitions[i]
				if scalar.Name == name {

					if scalar.Description != description {
						panic(fmt.Errorf("hasScalar: want(description): %s, got: %s", description, scalar.Description))
					}

					if directives != nil && len(directives) != len(parsedDefinitions.Directives) {
						panic(fmt.Errorf("hasScalar: want(directives): %+v, got: %+v", directives, parsedDefinitions.Directives))
					}

					for i, directive := range directives {
						d := parsedDefinitions.Directives[i]
						if d.Name != directive {
							panic(fmt.Errorf("hasScalar: want(directive): %s, got: %s", directive, d.Name))
						}
					}

					return
				}
			}

			panic(fmt.Errorf("hasScalar: want: %s (not found)", name))
		}
	}

	hasObjectTypeDefinition := func(name, description string, fields ...string) checkFunc {
		return func(definition document.TypeSystemDefinition, parsedDefinitions ParsedDefinitions, err error) {
			for _, i := range definition.ObjectTypeDefinitions {
				objectType := parsedDefinitions.ObjectTypeDefinitions[i]
				if objectType.Name == name {

					if objectType.Description != description {
						panic(fmt.Errorf("hasObjectTypeDefinition: want(description): %s, got: %s [for type: %s]", description, objectType.Description, name))
					}

					if fields != nil && len(objectType.FieldsDefinition) != len(fields) {
						panic(fmt.Errorf("hasObjectTypeDefinition: want(fields): %d, got: %d [for type: %s]", len(fields), len(objectType.FieldsDefinition), name))
					}

					for i, fieldName := range fields {
						index := objectType.FieldsDefinition[i]
						fieldObject := parsedDefinitions.FieldDefinitions[index]
						if fieldObject.Name != fieldName {
							panic(fmt.Errorf("hasObjectTypeDefinition: want(field): %s, got: %s [for type: %s]", fieldName, fieldObject.Name, name))
						}
					}

					return
				}
			}

			panic(fmt.Errorf("hasObjectTypeDefinition: want: %s (not found)", name))
		}
	}

	typeHasDirective := func(typeName, directiveName string) checkFunc {
		return func(definition document.TypeSystemDefinition, parsedDefinitions ParsedDefinitions, err error) {
			objectType, found := LookupObjectTypeDefinitionByName(typeName, parsedDefinitions)
			if !found {
				panic(fmt.Errorf("typeHasDirective: want(type): %s (not found)", typeName))
			}

			directives := LookupDirectivesByObjectType(objectType, parsedDefinitions)
			for _, directive := range directives {
				if directive.Name == directiveName {
					return
				}
			}

			panic(fmt.Errorf("typeHasDirective: want(directive): %s, got: %+v", directiveName, directives))
		}
	}

	hasInterface := func(name, description string, fields ...string) checkFunc {
		return func(definition document.TypeSystemDefinition, parsedDefinitions ParsedDefinitions, err error) {
			for _, i := range definition.InterfaceTypeDefinitions {
				interfaceTypeDefinition := parsedDefinitions.InterfaceTypeDefinitions[i]
				if interfaceTypeDefinition.Name == name {

					if interfaceTypeDefinition.Description != description {
						panic(fmt.Errorf("hasInterface: want(description): %s, got: %s [for type: %s]", description, interfaceTypeDefinition.Description, name))
					}

					if fields != nil && len(interfaceTypeDefinition.FieldsDefinition) != len(fields) {
						panic(fmt.Errorf("hasInterface: want(fields): %d, got: %d [for type: %s]", len(fields), len(interfaceTypeDefinition.FieldsDefinition), name))
					}

					for i, fieldName := range fields {
						index := interfaceTypeDefinition.FieldsDefinition[i]
						fieldObject := parsedDefinitions.FieldDefinitions[index]
						if fieldObject.Name != fieldName {
							panic(fmt.Errorf("hasInterface: want(field): %s, got: %s [for type: %s]", fieldName, fieldObject.Name, name))
						}
					}

					return
				}
			}

			panic(fmt.Errorf("hasInterface: want: %s (not found)", name))
		}
	}

	interfaceHasDirective := func(interfaceName, directiveName string) checkFunc {
		return func(definition document.TypeSystemDefinition, parsedDefinitions ParsedDefinitions, err error) {
			interfaceTypeDefinition, found := LookupInterfaceTypeDefinitionByName(interfaceName, parsedDefinitions)
			if !found {
				panic(fmt.Errorf("interfaceHasDirective: want(type): %s (not found)", interfaceName))
			}

			directives := LookupDirectivesByInterfaceType(interfaceTypeDefinition, parsedDefinitions)
			for _, directive := range directives {
				if directive.Name == directiveName {
					return
				}
			}

			panic(fmt.Errorf("interfaceHasDirective: want(directive): %s, got: %+v", directiveName, directives))
		}
	}

	hasEnumTypeDefinition := func(name, description string, values ...string) checkFunc {
		return func(definition document.TypeSystemDefinition, parsedDefinitions ParsedDefinitions, err error) {
			for _, i := range definition.EnumTypeDefinitions {
				enumTypeDefinition := parsedDefinitions.EnumTypeDefinitions[i]
				if enumTypeDefinition.Name == name {

					if enumTypeDefinition.Description != description {
						panic(fmt.Errorf("hasEnumTypeDefinition: want(description): %s, got: %s [for name: %s]", description, enumTypeDefinition.Description, name))
					}

					if values != nil && len(enumTypeDefinition.EnumValuesDefinition) != len(values) {
						panic(fmt.Errorf("hasEnumTypeDefinition: want(values): %d, got: %d [for name: %s]", len(values), len(enumTypeDefinition.EnumValuesDefinition), name))
					}

					for i, valueName := range values {
						index := enumTypeDefinition.EnumValuesDefinition[i]
						enumValue := parsedDefinitions.EnumValuesDefinitions[index]
						if enumValue.EnumValue != valueName {
							panic(fmt.Errorf("hasEnumTypeDefinition: want(value): %s, got: %s [for name: %s]", valueName, enumValue.EnumValue, name))
						}
					}

					return
				}
			}

			panic(fmt.Errorf("hasEnumTypeDefinition: want: %s (not found)", name))
		}
	}

	enumHasDirective := func(enumTypeDefinitionName, directiveName string) checkFunc {
		return func(definition document.TypeSystemDefinition, parsedDefinitions ParsedDefinitions, err error) {
			enumTypeDefinition, found := LookupEnumTypeDefinitionByName(enumTypeDefinitionName, parsedDefinitions)
			if !found {
				panic(fmt.Errorf("enumHasDirective: want(type): %s (not found)", enumTypeDefinitionName))
			}

			directives := LookupDirectivesByEnumType(enumTypeDefinition, parsedDefinitions)
			for _, directive := range directives {
				if directive.Name == directiveName {
					return
				}
			}

			panic(fmt.Errorf("enumHasDirective: want(directive): %s, got: %+v", directiveName, directives))
		}
	}

	hasInputTypeDefinition := func(name, description string, fields ...string) checkFunc {
		return func(definition document.TypeSystemDefinition, parsedDefinitions ParsedDefinitions, err error) {
			for _, i := range definition.InputObjectTypeDefinitions {
				inputObjectTypeDefinition := parsedDefinitions.InputObjectTypeDefinitions[i]
				if inputObjectTypeDefinition.Name == name {

					if inputObjectTypeDefinition.Description != description {
						panic(fmt.Errorf("hasInputTypeDefinition: want(description): %s, got: %s [for type: %s]", description, inputObjectTypeDefinition.Description, name))
					}

					if fields != nil && len(inputObjectTypeDefinition.InputFieldsDefinition) != len(fields) {
						panic(fmt.Errorf("hasInputTypeDefinition: want(fields): %d, got: %d [for type: %s]", len(fields), len(inputObjectTypeDefinition.InputFieldsDefinition), name))
					}

					for i, fieldName := range fields {
						index := inputObjectTypeDefinition.InputFieldsDefinition[i]
						fieldObject := parsedDefinitions.InputValueDefinitions[index]
						if fieldObject.Name != fieldName {
							panic(fmt.Errorf("hasInputTypeDefinition: want(field): %s, got: %s [for type: %s]", fieldName, fieldObject.Name, name))
						}
					}

					return
				}
			}

			panic(fmt.Errorf("hasInputTypeDefinition: want: %s (not found)", name))
		}
	}

	inputObjectTypeHasDirective := func(inputTypeName, directiveName string) checkFunc {
		return func(definition document.TypeSystemDefinition, parsedDefinitions ParsedDefinitions, err error) {
			inputObjectTypeDefinition, found := LookupInputTypeDefinitionByName(inputTypeName, parsedDefinitions)
			if !found {
				panic(fmt.Errorf("inputObjectTypeHasDirective: want(type): %s (not found)", inputTypeName))
			}

			directives := LookupDirectivesByInputType(inputObjectTypeDefinition, parsedDefinitions)
			for _, directive := range directives {
				if directive.Name == directiveName {
					return
				}
			}

			panic(fmt.Errorf("inputObjectTypeHasDirective: want(directive): %s, got: %+v", directiveName, directives))
		}
	}

	hasDirectiveDefinition := func(name, description string, locations ...document.DirectiveLocation) checkFunc {
		return func(definition document.TypeSystemDefinition, parsedDefinitions ParsedDefinitions, err error) {
			directive, found := LookupDirectiveDefinitionByName(name, parsedDefinitions)
			if !found {
				panic(fmt.Errorf("hasDirectiveDefinition: want: %s (not found)", name))
			}

			if locations != nil && len(locations) != len(directive.DirectiveLocations) {
				panic(fmt.Errorf("hasDirectiveDefinition: want(locations): %d, got: %d", len(locations), len(directive.DirectiveLocations)))
			}

			for i, location := range locations {
				if directive.DirectiveLocations[i] != location {
					panic(fmt.Errorf("hasDirectiveDefinition: want(location): %s, got: %s", location, directive.DirectiveLocations[i]))
				}
			}
		}
	}

	t.Run("UnionTypeDefinitions", func(t *testing.T) {
		run(t, Case{
			input: `
				"unifies SearchResult"
				union SearchResult = Photo | Person
				union thirdUnion 
				"second union"
				union secondUnion
				union firstUnion @fromTop(to: "bottom")
				"unifies UnionExample"
				union UnionExample = First | Second
				`,
			checks: checks(
				hasNoError(),
				unionsParsed(5),
				hasUnion("thirdUnion", ""),
				hasUnion("SearchResult", "unifies SearchResult", "Photo", "Person"),
				hasUnion("secondUnion", "second union"),
				hasUnion("firstUnion", ""),
				unionHasDirective("firstUnion", "fromTop"),
				hasUnion("UnionExample", "unifies UnionExample", "First", "Second"),
			),
		})
	})

	t.Run("Schema", func(t *testing.T) {
		run(t, Case{
			input: `
				schema {
					query: Query
					mutation: Mutation
				}
				`,
			checks: checks(
				hasNoError(),
				hasSchema("Query", "Mutation", ""),
			),
		})
	})

	t.Run("Schema error case", func(t *testing.T) {
		run(t, Case{
			input: `
				schema {
					query: Query
					mutation: Mutation
				}

				schema {
					query: Query
					mutation: Mutation
				}
				`,
			checks: checks(
				hasError(),
			),
		})
	})

	t.Run("ScalarTypeDefinitions", func(t *testing.T) {
		run(t, Case{
			input: `
				"this is a scalar"
				scalar JSON
				
				scalar testName @fromTop(to: "bottom")
				
				"this is another scalar" scalar XML
				`,
			checks: checks(
				hasNoError(),
				hasScalar("JSON", "this is a scalar"),
				hasScalar("testName", "", "fromTop"),
				hasScalar("XML", "this is another scalar"),
			),
		})
	})

	t.Run("ObjectTypeDefinitions", func(t *testing.T) {
		run(t, Case{
			input: `
				"this is a Person"
				type Person {
					name: String
				}
				type testType
				"second Type" type secondType
				type thirdType @fromTop(to: "bottom")
				"this is an Animal"
				type Animal {
					age: Int
				}
				`,
			checks: checks(
				hasNoError(),
				hasObjectTypeDefinition("Person", "this is a Person", "name"),
				hasObjectTypeDefinition("testType", ""),
				hasObjectTypeDefinition("secondType", "second Type"),
				hasObjectTypeDefinition("thirdType", ""),
				typeHasDirective("thirdType", "fromTop"),
				hasObjectTypeDefinition("Animal", "this is an Animal", "age"),
			),
		})
	})

	t.Run("InterfaceTypeDefinitions", func(t *testing.T) {
		run(t, Case{
			input: `
				"describes firstEntity"
				interface firstEntity {
					name: String
				}
				interface firstInterface
				"second interface"
				interface secondInterface
				interface thirdInterface @fromTop(to: "bottom")
				"describes secondEntity"
				interface secondEntity {
					age: Int
				}
				`,
			checks: checks(
				hasNoError(),
				hasInterface("firstEntity", "describes firstEntity", "name"),
				hasInterface("firstInterface", ""),
				hasInterface("secondInterface", "second interface"),
				hasInterface("thirdInterface", ""),
				interfaceHasDirective("thirdInterface", "fromTop"),
				hasInterface("secondEntity", "describes secondEntity", "age"),
			),
		})
	})

	t.Run("EnumTypeDefinitions", func(t *testing.T) {
		run(t, Case{
			input: `
				"describes direction"
				enum Direction {
				  NORTH
				}
				enum thirdEnum
				"second enum"
				enum secondEnum
				enum firstEnum @fromTop(to: "bottom")
				"enumerates EnumExample"
				enum EnumExample {
					NORTH
					SOUTH
			
					WEST
					EAST
				}
				`,
			checks: checks(
				hasNoError(),
				hasEnumTypeDefinition("Direction", "describes direction", "NORTH"),
				hasEnumTypeDefinition("thirdEnum", ""),
				hasEnumTypeDefinition("secondEnum", "second enum"),
				hasEnumTypeDefinition("firstEnum", ""),
				enumHasDirective("firstEnum", "fromTop"),
				hasEnumTypeDefinition("EnumExample", "enumerates EnumExample", "NORTH", "SOUTH", "WEST", "EAST"),
			),
		})
	})

	t.Run("InputObjectTypeDefinitions", func(t *testing.T) {
		run(t, Case{
			input: `
				"describes Person"
				input Person {
					name: String
				}
				input thirdInput
				"second input"
				input secondInput
				input firstInput @fromTop(to: "bottom")
				"inputs InputExample"
				input InputExample {
					name: String
					age: Int
				}
				`,
			checks: checks(
				hasNoError(),
				hasInputTypeDefinition("Person", "describes Person", "name"),
				hasInputTypeDefinition("thirdInput", ""),
				hasInputTypeDefinition("secondInput", "second input"),
				hasInputTypeDefinition("firstInput", ""),
				inputObjectTypeHasDirective("firstInput", "fromTop"),
				hasInputTypeDefinition("InputExample", "inputs InputExample", "name", "age"),
			),
		})
	})

	t.Run("DirectiveDefinitions", func(t *testing.T) {
		run(t, Case{
			input: `
				"describes somewhere"
				directive @ somewhere on QUERY
				directive @ somehow on MUTATION

				"describes someway"
				directive @ someway on SUBSCRIPTION | MUTATION
				`,
			checks: checks(
				hasNoError(),
				hasDirectiveDefinition("somewhere", "describes somewhere", document.DirectiveLocationQUERY),
				hasDirectiveDefinition("somehow", "", document.DirectiveLocationMUTATION),
				hasDirectiveDefinition("someway", "describes someway", document.DirectiveLocationSUBSCRIPTION, document.DirectiveLocationMUTATION),
			),
		})
	})

	t.Run("Starwars", func(t *testing.T) {
		run(t, Case{
			input: `
		schema {
		  query: Query
		  mutation: Mutation
		  subscription: Subscription
		}

		"The query type, represents all of the entry points into our object graph"
		type Query {
		  hero(episode: Episode): Character
		  reviews(episode: Episode!): [Review]
		  search(text: String): [SearchResult]
		  character(id: ID!): Character
		  droid(id: ID!): Droid
		  human(id: ID!): Human
		  starship(id: ID!): Starship
		}

		"The mutation type, represents all updates we can make to our data"
		type Mutation {
		  createReview(episode: Episode, review: ReviewInput!): Review
		}

		"The subscription type, represents all subscriptions we can make to our data"
		type Subscription {
		  reviewAdded(episode: Episode): Review
		}

		"The episodes in the Star Wars trilogy"
		enum Episode {
		  "Star Wars Episode IV: A New Hope, released in 1977."
		  NEWHOPE
		  "Star Wars Episode V: The Empire Strikes Back, released in 1980."
		  EMPIRE
		  "Star Wars Episode VI: Return of the Jedi, released in 1983."
		  JEDI
		}

		"A character from the Star Wars universe"
		interface Character {
		  "The ID of the character"
		  id: ID!
		  "The name of the character"
		  name: String!
		  "The friends of the character, or an empty list if they have none"
		  friends: [Character]
		  "The friends of the character exposed as a connection with edges"
		  friendsConnection(first: Int, after: ID): FriendsConnection!
		  "The movies this character appears in"
		  appearsIn: [Episode]!
		}

		"Units of height"
		enum LengthUnit {
		  "The standard unit around the world"
		  METER
		  "Primarily used in the United States"
		  FOOT
		}

		"A humanoid creature from the Star Wars universe"
		type Human implements Character {
		  "The ID of the human"
		  id: ID!
		  "What this human calls themselves"
		  name: String!
		  "The home planet of the human, or null if unknown"
		  homePlanet: String
		  "Height in the preferred unit, default is meters"
		  height(unit: LengthUnit = METER): Float
		  "Mass in kilograms, or null if unknown"
		  mass: Float
		  "This human's friends, or an empty list if they have none"
		  friends: [Character]
		  "The friends of the human exposed as a connection with edges"
		  friendsConnection(first: Int, after: ID): FriendsConnection!
		  "The movies this human appears in"
		  appearsIn: [Episode]!
		  "A list of starships this person has piloted, or an empty list if none"
		  starships: [Starship]
		}

		"An autonomous mechanical character in the Star Wars universe"
		type Droid implements Character {
		  "The ID of the droid"
		  id: ID!
		  "What others call this droid"
		  name: String!
		  "This droid's friends, or an empty list if they have none"
		  friends: [Character]
		  "The friends of the droid exposed as a connection with edges"
		  friendsConnection(first: Int, after: ID): FriendsConnection!
		  "The movies this droid appears in"
		  appearsIn: [Episode]!
		  "This droid's primary function"
		  primaryFunction: String
		}

		"A connection object for a character's friends"
		type FriendsConnection {
		  "The total number of friends"
		  totalCount: Int
		  "The edges for each of the character's friends."
		  edges: [FriendsEdge]
		  "A list of the friends, as a convenience when edges are not needed."
		  friends: [Character]
		  "Information for paginating this connection"
		  pageInfo: PageInfo!
		}

		"An edge object for a character's friends"
		type FriendsEdge {
		  "A cursor used for pagination"
		  cursor: ID!
		  "The character represented by this friendship edge"
		  node: Character
		}

		"Information for paginating this connection"
		type PageInfo {
		  startCursor: ID
		  endCursor: ID
		  hasNextPage: Boolean!
		}

		"Represents a review for a movie"
		type Review {
		  "The movie"
		  episode: Episode
		  "The number of stars this review gave, 1-5"
		  stars: Int!
		  "Comment about the movie"
		  commentary: String
		}

		"The input object sent when someone is creating a new review"
		input ReviewInput {
		  "0-5 stars"
		  stars: Int!
		  "Comment about the movie, optional"
		  commentary: String
		  "Favorite color, optional"
		  favorite_color: ColorInput
		}

		"The input object sent when passing in a color"
		input ColorInput {
		  red: Int!
		  green: Int!
		  blue: Int!
		}

		type Starship {
		  "The ID of the starship"
		  id: ID!
		  "The name of the starship"
		  name: String!
		  "Length of the starship, along the longest axis"
		  length(unit: LengthUnit = METER): Float
		  coordinates: [[Float!]!]
		}

		union SearchResult = Human | Droid | Starship
		`,
			checks: checks(
				hasNoError(),
				hasSchema("Query", "Mutation", "Subscription"),
				hasObjectTypeDefinition("Query",
					"The query type, represents all of the entry points into our object graph",
					"hero", "reviews", "search", "character", "droid", "human", "starship"),
				hasObjectTypeDefinition("Starship", "", "id", "name", "length", "coordinates"),
			),
		})
	})
}

/*func TestTypeSystemDefinition(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parseTypeSystemDefinition", func() {
		tests := []struct {
			it                         string
			input                      string
			expectErr                  types.GomegaMatcher
			expectTypeSystemDefinition types.GomegaMatcher
			expectParsedDefinitions    types.GomegaMatcher
		}{
			{
				it: "should parse simple TypeSystemDefinition with a Schema definition",
				input: `
				schema {
					query: Query
					mutation: Mutation
				}
				`,
				expectErr: BeNil(),
				expectTypeSystemDefinition: Equal(document.TypeSystemDefinition{
					DirectiveDefinitions:       []int{},
					UnionTypeDefinitions:       []int{},
					InputObjectTypeDefinitions: []int{},
					EnumTypeDefinitions:        []int{},
					InterfaceTypeDefinitions:   []int{},
					ObjectTypeDefinitions:      []int{},
					ScalarTypeDefinitions:      []int{},
					SchemaDefinition: document.SchemaDefinition{
						Query:      "Query",
						Mutation:   "Mutation",
						Directives: []int{},
					}}),
			},
			{
				it: "should parse simple TypeSystemDefinition with ScalarTypeDefinitions",
				input: `
				"this is a scalar" scalar JSON
				scalar testName @fromTop(to: "bottom")
				"this is another scalar" scalar XML
				`,
				expectErr: BeNil(),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Directives: document.Directives{
						{
							Name:      "fromTop",
							Arguments: []int{0},
						},
					},
					Arguments: document.Arguments{
						{
							Name: "to",
							Value: document.Value{
								ValueType:   document.ValueTypeString,
								StringValue: "bottom",
							},
						},
					},
					ScalarTypeDefinitions: document.ScalarTypeDefinitions{
						{
							Name:        "JSON",
							Description: "this is a scalar",
							Directives:  []int{},
						},
						{
							Name:       "testName",
							Directives: []int{0},
						},
						{
							Name:        "XML",
							Description: "this is another scalar",
							Directives:  []int{},
						},
					},
				}.initEmptySlices()),
				expectTypeSystemDefinition: Equal(document.TypeSystemDefinition{
					DirectiveDefinitions:       []int{},
					UnionTypeDefinitions:       []int{},
					InputObjectTypeDefinitions: []int{},
					EnumTypeDefinitions:        []int{},
					InterfaceTypeDefinitions:   []int{},
					ObjectTypeDefinitions:      []int{},
					ScalarTypeDefinitions:      []int{0, 1, 2}}),
			},
			{
				it: "should parse simple TypeSystemDefinition with ObjectTypeDefinitions",
				input: `
				"this is a Person"
				type Person {
					name: String
				}
				type testType
				"second Type" type secondType
				type thirdType @fromTop(to: "bottom")
				"this is an Animal"
				type Animal {
					age: Int
				}
				`,
				expectErr: BeNil(),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Directives: document.Directives{
						{
							Name:      "fromTop",
							Arguments: []int{0},
						},
					},
					Arguments: document.Arguments{
						{
							Name: "to",
							Value: document.Value{
								ValueType:   document.ValueTypeString,
								StringValue: "bottom",
							},
						},
					},
					FieldDefinitions: document.FieldDefinitions{
						{
							Name:                "name",
							Directives:          []int{},
							ArgumentsDefinition: []int{},
							Type: document.NamedType{
								Name: "String",
							},
						},
						{
							Name:                "age",
							ArgumentsDefinition: []int{},
							Directives:          []int{},
							Type: document.NamedType{
								Name: "Int",
							},
						},
					},
					ObjectTypeDefinitions: document.ObjectTypeDefinitions{
						{
							Name:             "Person",
							Description:      "this is a Person",
							FieldsDefinition: []int{0},
							Directives:       []int{},
						},
						{
							Name:             "testType",
							Directives:       []int{},
							FieldsDefinition: []int{},
						},
						{
							Name:             "secondType",
							Description:      "second Type",
							FieldsDefinition: []int{},
							Directives:       []int{},
						},
						{
							Name:             "thirdType",
							Directives:       []int{0},
							FieldsDefinition: []int{},
						},
						{
							Name:             "Animal",
							Description:      "this is an Animal",
							FieldsDefinition: []int{1},
							Directives:       []int{},
						},
					},
				}.initEmptySlices()),
				expectTypeSystemDefinition: Equal(document.TypeSystemDefinition{
					DirectiveDefinitions:       []int{},
					UnionTypeDefinitions:       []int{},
					InputObjectTypeDefinitions: []int{},
					EnumTypeDefinitions:        []int{},
					InterfaceTypeDefinitions:   []int{},
					ScalarTypeDefinitions:      []int{},
					ObjectTypeDefinitions:      []int{0, 1, 2, 3, 4}}),
			},
			{
				it: "should parse simple TypeSystemDefinition with InterfaceTypeDefinitions",
				input: `
				"describes firstEntity"
				interface firstEntity {
					name: String
				}
				interface firstInterface
				"second interface"
				interface secondInterface
				interface thirdInterface @fromTop(to: "bottom")
				"describes secondEntity"
				interface secondEntity {
					age: Int
				}
				`,
				expectErr: BeNil(),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Arguments: document.Arguments{
						{
							Name: "to",
							Value: document.Value{
								ValueType:   document.ValueTypeString,
								StringValue: "bottom",
							},
						},
					},
					Directives: document.Directives{
						{
							Name:      "fromTop",
							Arguments: []int{0},
						},
					},
					FieldDefinitions: document.FieldDefinitions{
						{
							Name:                "name",
							Directives:          []int{},
							ArgumentsDefinition: []int{},
							Type: document.NamedType{
								Name: "String",
							},
						},
						{
							Name:                "age",
							ArgumentsDefinition: []int{},
							Directives:          []int{},
							Type: document.NamedType{
								Name: "Int",
							},
						},
					},
					InterfaceTypeDefinitions: document.InterfaceTypeDefinitions{
						{
							Name:             "firstEntity",
							Description:      "describes firstEntity",
							FieldsDefinition: []int{0},
							Directives:       []int{},
						},
						{
							Name:             "firstInterface",
							Directives:       []int{},
							FieldsDefinition: []int{},
						},
						{
							Name:             "secondInterface",
							Description:      "second interface",
							FieldsDefinition: []int{},
							Directives:       []int{},
						},
						{
							Name:             "thirdInterface",
							Directives:       []int{0},
							FieldsDefinition: []int{},
						},
						{
							Name:             "secondEntity",
							Description:      "describes secondEntity",
							FieldsDefinition: []int{1},
							Directives:       []int{},
						},
					},
				}.initEmptySlices()),
				expectTypeSystemDefinition: Equal(document.TypeSystemDefinition{
					DirectiveDefinitions:       []int{},
					UnionTypeDefinitions:       []int{},
					InputObjectTypeDefinitions: []int{},
					EnumTypeDefinitions:        []int{},
					ObjectTypeDefinitions:      []int{},
					ScalarTypeDefinitions:      []int{},
					InterfaceTypeDefinitions:   []int{0, 1, 2, 3, 4},
				}),
			},
			{
				it: "should parse simple TypeSystemDefinition with UnionTypeDefinitions",
				input: `
				"unifies SearchResult"
				union SearchResult = Photo | Person
				union thirdUnion
				"second union"
				union secondUnion
				union firstUnion @fromTop(to: "bottom")
				"unifies UnionExample"
				union UnionExample = First | Second
				`,
				expectErr: BeNil(),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Arguments: document.Arguments{
						{
							Name: "to",
							Value: document.Value{
								ValueType:   document.ValueTypeString,
								StringValue: "bottom",
							},
						},
					},
					Directives: document.Directives{
						{
							Name:      "fromTop",
							Arguments: []int{0},
						},
					},
					UnionTypeDefinitions: document.UnionTypeDefinitions{
						{
							Name:        "SearchResult",
							Description: "unifies SearchResult",
							UnionMemberTypes: document.UnionMemberTypes{
								"Photo",
								"Person",
							},
							Directives: []int{},
						},
						{
							Name:       "thirdUnion",
							Directives: []int{},
						},
						{
							Name:        "secondUnion",
							Description: "second union",
							Directives:  []int{},
						},
						{
							Name:       "firstUnion",
							Directives: []int{0},
						},
						{
							Name:        "UnionExample",
							Description: "unifies UnionExample",
							UnionMemberTypes: document.UnionMemberTypes{
								"First",
								"Second",
							},
							Directives: []int{},
						},
					},
				}.initEmptySlices()),
				expectTypeSystemDefinition: Equal(document.TypeSystemDefinition{
					DirectiveDefinitions:       []int{},
					InputObjectTypeDefinitions: []int{},
					EnumTypeDefinitions:        []int{},
					InterfaceTypeDefinitions:   []int{},
					ObjectTypeDefinitions:      []int{},
					ScalarTypeDefinitions:      []int{},
					UnionTypeDefinitions:       []int{0, 1, 2, 3, 4}}),
			},
			{
				it: "should parse simple TypeSystemDefinition with EnumTypeDefinitions",
				input: `
				"describes direction"
				enum Direction {
				  NORTH
				}
				enum thirdEnum
				"second enum"
				enum secondEnum
				enum firstEnum @fromTop(to: "bottom")
				"enumerates EnumExample"
				enum EnumExample {
				  NORTH
				}
				`,
				expectErr: BeNil(),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Arguments: document.Arguments{
						{
							Name: "to",
							Value: document.Value{
								ValueType:   document.ValueTypeString,
								StringValue: "bottom",
							},
						},
					},
					Directives: document.Directives{
						{
							Name:      "fromTop",
							Arguments: []int{0},
						},
					},
					EnumValuesDefinitions: document.EnumValueDefinitions{
						{
							EnumValue:  "NORTH",
							Directives: []int{},
						},
						{
							EnumValue:  "NORTH",
							Directives: []int{},
						},
					},
					EnumTypeDefinitions: document.EnumTypeDefinitions{
						{
							Name:                 "Direction",
							Description:          "describes direction",
							EnumValuesDefinition: []int{0},
							Directives:           []int{},
						},
						{
							Name:                 "thirdEnum",
							Directives:           []int{},
							EnumValuesDefinition: []int{},
						},
						{
							Name:                 "secondEnum",
							Description:          "second enum",
							EnumValuesDefinition: []int{},
							Directives:           []int{},
						},
						{
							Name:                 "firstEnum",
							Directives:           []int{0},
							EnumValuesDefinition: []int{},
						},
						{
							Name:                 "EnumExample",
							Description:          "enumerates EnumExample",
							EnumValuesDefinition: []int{1},
							Directives:           []int{},
						},
					},
				}.initEmptySlices()),
				expectTypeSystemDefinition: Equal(document.TypeSystemDefinition{
					DirectiveDefinitions:       []int{},
					UnionTypeDefinitions:       []int{},
					InputObjectTypeDefinitions: []int{},
					InterfaceTypeDefinitions:   []int{},
					ObjectTypeDefinitions:      []int{},
					ScalarTypeDefinitions:      []int{},
					EnumTypeDefinitions:        []int{0, 1, 2, 3, 4}}),
			},
			{
				it: "should parse simple TypeSystemDefinition with InputObjectTypeDefinitions",
				input: `
				"describes Person"
				input Person {
					name: String
				}
				input thirdInput
				"second input"
				input secondInput
				input firstInput @fromTop(to: "bottom")
				"inputs InputExample"
				input InputExample {
					name: String
				}
				`,
				expectErr: BeNil(),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					Directives: document.Directives{
						{
							Name:      "fromTop",
							Arguments: []int{0},
						},
					},
					Arguments: document.Arguments{
						{
							Name: "to",
							Value: document.Value{
								ValueType:   document.ValueTypeString,
								StringValue: "bottom",
							},
						},
					},
					InputValueDefinitions: document.InputValueDefinitions{
						{
							Name: "name",
							Type: document.NamedType{
								Name: "String",
							},
							Directives: []int{},
						},
						{
							Name: "name",
							Type: document.NamedType{
								Name: "String",
							},
							Directives: []int{},
						},
					},
					InputObjectTypeDefinitions: document.InputObjectTypeDefinitions{
						{
							Name:                  "Person",
							Description:           "describes Person",
							InputFieldsDefinition: []int{0},
							Directives:            []int{},
						},
						{
							Name:                  "thirdInput",
							Directives:            []int{},
							InputFieldsDefinition: []int{},
						},
						{
							Name:                  "secondInput",
							Description:           "second input",
							InputFieldsDefinition: []int{},
							Directives:            []int{},
						},
						{
							Name:                  "firstInput",
							Directives:            []int{0},
							InputFieldsDefinition: []int{},
						},
						{
							Name:                  "InputExample",
							Description:           "inputs InputExample",
							InputFieldsDefinition: []int{1},
							Directives:            []int{},
						},
					},
				}.initEmptySlices()),
				expectTypeSystemDefinition: Equal(document.TypeSystemDefinition{
					DirectiveDefinitions:       []int{},
					UnionTypeDefinitions:       []int{},
					EnumTypeDefinitions:        []int{},
					InterfaceTypeDefinitions:   []int{},
					ObjectTypeDefinitions:      []int{},
					ScalarTypeDefinitions:      []int{},
					InputObjectTypeDefinitions: []int{0, 1, 2, 3, 4}}),
			},
			{
				it: "should parse simple TypeSystemDefinition with DirectiveDefinitions",
				input: `
				"describes somewhere"
				directive @ somewhere on QUERY
				directive @ somehow on MUTATION

				"describes someway"
				directive @ someway on SUBSCRIPTION
				`,
				expectErr: BeNil(),
				expectParsedDefinitions: Equal(ParsedDefinitions{
					DirectiveDefinitions: document.DirectiveDefinitions{
						{
							Name:        "somewhere",
							Description: "describes somewhere",
							DirectiveLocations: document.DirectiveLocations{
								document.DirectiveLocationQUERY,
							},
							ArgumentsDefinition: []int{},
						},
						{
							Name: "somehow",
							DirectiveLocations: document.DirectiveLocations{
								document.DirectiveLocationMUTATION,
							},
							ArgumentsDefinition: []int{},
						},
						{
							Name:        "someway",
							Description: "describes someway",
							DirectiveLocations: document.DirectiveLocations{
								document.DirectiveLocationSUBSCRIPTION,
							},
							ArgumentsDefinition: []int{},
						},
					},
				}.initEmptySlices()),
				expectTypeSystemDefinition: Equal(document.TypeSystemDefinition{
					UnionTypeDefinitions:       []int{},
					InputObjectTypeDefinitions: []int{},
					EnumTypeDefinitions:        []int{},
					InterfaceTypeDefinitions:   []int{},
					ObjectTypeDefinitions:      []int{},
					ScalarTypeDefinitions:      []int{},
					DirectiveDefinitions:       []int{0, 1, 2}}),
			},
			{
				it: "should not parse TypeSystemDefinition when type has no valid TypeSystemDefinition identifier",
				input: `
				"describes nonsense"
				nonsense @ somewhere on QUERY
				`,
				expectErr: Not(BeNil()),
				expectTypeSystemDefinition: Equal(document.TypeSystemDefinition{
					DirectiveDefinitions:       []int{},
					UnionTypeDefinitions:       []int{},
					InputObjectTypeDefinitions: []int{},
					EnumTypeDefinitions:        []int{},
					InterfaceTypeDefinitions:   []int{},
					ObjectTypeDefinitions:      []int{},
					ScalarTypeDefinitions:      []int{},
				}),
			},
			{
				it: "should not parse when SchemaDefinition is defined more than once",
				input: `
				schema {
					query: Query
					mutation: Mutation
				}

				schema {
					query: ThisShouldntBeHere
					mutation: ThisShouldntBeHereEither
				}
				`,
				expectErr: Not(BeNil()),
				expectTypeSystemDefinition: Equal(document.TypeSystemDefinition{
					DirectiveDefinitions:       []int{},
					UnionTypeDefinitions:       []int{},
					InputObjectTypeDefinitions: []int{},
					EnumTypeDefinitions:        []int{},
					InterfaceTypeDefinitions:   []int{},
					ObjectTypeDefinitions:      []int{},
					ScalarTypeDefinitions:      []int{},
					SchemaDefinition: document.SchemaDefinition{
						Query:      "Query",
						Mutation:   "Mutation",
						Directives: []int{},
					}}),
			},
		}
		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				parser := NewParser()
				parser.l.SetInput(test.input)

				val, err := parser.parseTypeSystemDefinition()
				Expect(val).To(test.expectTypeSystemDefinition)
				Expect(err).To(test.expectErr)
				if test.expectParsedDefinitions != nil {
					Expect(parser.ParsedDefinitions).To(test.expectParsedDefinitions)
				}

			})
		}
	})
}*/

/*	starWarsSchema := `
		schema {
		  query: Query
		  mutation: Mutation
		  subscription: Subscription
		}

		"The query type, represents all of the entry points into our object graph"
		type Query {
		  hero(episode: Episode): Character
		  reviews(episode: Episode!): [Review]
		  search(text: String): [SearchResult]
		  character(id: ID!): Character
		  droid(id: ID!): Droid
		  human(id: ID!): Human
		  starship(id: ID!): Starship
		}

		"The mutation type, represents all updates we can make to our data"
		type Mutation {
		  createReview(episode: Episode, review: ReviewInput!): Review
		}

		"The subscription type, represents all subscriptions we can make to our data"
		type Subscription {
		  reviewAdded(episode: Episode): Review
		}

		"The episodes in the Star Wars trilogy"
		enum Episode {
		  "Star Wars Episode IV: A New Hope, released in 1977."
		  NEWHOPE
		  "Star Wars Episode V: The Empire Strikes Back, released in 1980."
		  EMPIRE
		  "Star Wars Episode VI: Return of the Jedi, released in 1983."
		  JEDI
		}

		"A character from the Star Wars universe"
		interface Character {
		  "The ID of the character"
		  id: ID!
		  "The name of the character"
		  name: String!
		  "The friends of the character, or an empty list if they have none"
		  friends: [Character]
		  "The friends of the character exposed as a connection with edges"
		  friendsConnection(first: Int, after: ID): FriendsConnection!
		  "The movies this character appears in"
		  appearsIn: [Episode]!
		}

		"Units of height"
		enum LengthUnit {
		  "The standard unit around the world"
		  METER
		  "Primarily used in the United States"
		  FOOT
		}

		"A humanoid creature from the Star Wars universe"
		type Human implements Character {
		  "The ID of the human"
		  id: ID!
		  "What this human calls themselves"
		  name: String!
		  "The home planet of the human, or null if unknown"
		  homePlanet: String
		  "Height in the preferred unit, default is meters"
		  height(unit: LengthUnit = METER): Float
		  "Mass in kilograms, or null if unknown"
		  mass: Float
		  "This human's friends, or an empty list if they have none"
		  friends: [Character]
		  "The friends of the human exposed as a connection with edges"
		  friendsConnection(first: Int, after: ID): FriendsConnection!
		  "The movies this human appears in"
		  appearsIn: [Episode]!
		  "A list of starships this person has piloted, or an empty list if none"
		  starships: [Starship]
		}

		"An autonomous mechanical character in the Star Wars universe"
		type Droid implements Character {
		  "The ID of the droid"
		  id: ID!
		  "What others call this droid"
		  name: String!
		  "This droid's friends, or an empty list if they have none"
		  friends: [Character]
		  "The friends of the droid exposed as a connection with edges"
		  friendsConnection(first: Int, after: ID): FriendsConnection!
		  "The movies this droid appears in"
		  appearsIn: [Episode]!
		  "This droid's primary function"
		  primaryFunction: String
		}

		"A connection object for a character's friends"
		type FriendsConnection {
		  "The total number of friends"
		  totalCount: Int
		  "The edges for each of the character's friends."
		  edges: [FriendsEdge]
		  "A list of the friends, as a convenience when edges are not needed."
		  friends: [Character]
		  "Information for paginating this connection"
		  pageInfo: PageInfo!
		}

		"An edge object for a character's friends"
		type FriendsEdge {
		  "A cursor used for pagination"
		  cursor: ID!
		  "The character represented by this friendship edge"
		  node: Character
		}

		"Information for paginating this connection"
		type PageInfo {
		  startCursor: ID
		  endCursor: ID
		  hasNextPage: Boolean!
		}

		"Represents a review for a movie"
		type Review {
		  "The movie"
		  episode: Episode
		  "The number of stars this review gave, 1-5"
		  stars: Int!
		  "Comment about the movie"
		  commentary: String
		}

		"The input object sent when someone is creating a new review"
		input ReviewInput {
		  "0-5 stars"
		  stars: Int!
		  "Comment about the movie, optional"
		  commentary: String
		  "Favorite color, optional"
		  favorite_color: ColorInput
		}

		"The input object sent when passing in a color"
		input ColorInput {
		  red: Int!
		  green: Int!
		  blue: Int!
		}

		type Starship {
		  "The ID of the starship"
		  id: ID!
		  "The name of the starship"
		  name: String!
		  "Length of the starship, along the longest axis"
		  length(unit: LengthUnit = METER): Float
		  coordinates: [[Float!]!]
		}

		union SearchResult = Human | Droid | Starship
		`

		g.Describe("StarWars Schema", func() {
			g.It("should parse the starwars schema", func() {

				parser := NewParser()
				parser.l.SetInput(starWarsSchema)

				val, err := parser.parseTypeSystemDefinition()
				Expect(val).To(Equal(document.TypeSystemDefinition{
					SchemaDefinition: document.SchemaDefinition{
						Query:        "Query",
						Mutation:     "Mutation",
						Subscription: "Subscription",
					},
					ScalarTypeDefinitions: nil,
					ObjectTypeDefinitions: []document.ObjectTypeDefinition{
						{
							Description: "The query type, represents all of the entry points into our object graph",
							Name:        "Query",
							FieldsDefinition: []document.FieldDefinition{
								{
									Name: "hero",
									ArgumentsDefinition: document.ArgumentsDefinition{
										{
											Name: "episode",
											Type: document.NamedType{Name: "Episode"},
										},
									},
									Type: document.NamedType{Name: "Character"},
								},
								{
									Name: "reviews",
									ArgumentsDefinition: document.ArgumentsDefinition{
										{
											Name: "episode",
											Type: document.NamedType{Name: "Episode", NonNull: true},
										},
									},
									Type: document.ListType{Type: document.NamedType{Name: "Review"}},
								},
								{
									Name: "search",
									ArgumentsDefinition: document.ArgumentsDefinition{
										{
											Name: "text",
											Type: document.NamedType{Name: "String"},
										},
									},
									Type: document.ListType{Type: document.NamedType{Name: "SearchResult"}},
								},
								{
									Name: "character",
									ArgumentsDefinition: document.ArgumentsDefinition{
										{
											Name: "id",
											Type: document.NamedType{Name: "ID", NonNull: true},
										},
									},
									Type: document.NamedType{Name: "Character"},
								},
								{
									Name: "droid",
									ArgumentsDefinition: document.ArgumentsDefinition{
										{
											Name: "id",
											Type: document.NamedType{Name: "ID", NonNull: true},
										},
									},
									Type: document.NamedType{Name: "Droid"},
								},
								{
									Name: "human",
									ArgumentsDefinition: document.ArgumentsDefinition{
										{
											Name: "id",
											Type: document.NamedType{Name: "ID", NonNull: true},
										},
									},
									Type: document.NamedType{Name: "Human"},
								},
								{
									Name: "starship",
									ArgumentsDefinition: document.ArgumentsDefinition{
										{
											Name: "id",
											Type: document.NamedType{Name: "ID", NonNull: true},
										},
									},
									Type: document.NamedType{Name: "Starship"},
								},
							},
							ImplementsInterfaces: nil,
							Directives:           nil,
						},
						{
							Description: "The mutation type, represents all updates we can make to our data",
							Name:        "Mutation",
							FieldsDefinition: document.FieldDefinitions{
								{
									Name: "createReview",
									ArgumentsDefinition: document.ArgumentsDefinition{
										{
											Name: "episode",
											Type: document.NamedType{Name: "Episode"},
										},
										{
											Name: "review",
											Type: document.NamedType{Name: "ReviewInput", NonNull: true},
										},
									},
									Type: document.NamedType{Name: "Review"},
								},
							},
							ImplementsInterfaces: nil,
							Directives:           nil,
						},
						{
							Description: "The subscription type, represents all subscriptions we can make to our data",
							Name:        "Subscription",
							FieldsDefinition: document.FieldDefinitions{
								{
									Name: "reviewAdded",
									ArgumentsDefinition: document.ArgumentsDefinition{
										{
											Name: "episode",
											Type: document.NamedType{Name: "Episode"},
										},
									},
									Type: document.NamedType{Name: "Review"},
								},
							},
							ImplementsInterfaces: nil,
							Directives:           nil,
						},
						{
							Description: "A humanoid creature from the Star Wars universe",
							Name:        "Human",
							FieldsDefinition: document.FieldDefinitions{
								{
									Description:         "The ID of the human",
									Name:                "id",
									ArgumentsDefinition: nil,
									Type:                document.NamedType{Name: "ID", NonNull: true},
									Directives:          nil,
								},
								{
									Description:         "What this human calls themselves",
									Name:                "name",
									ArgumentsDefinition: nil,
									Type:                document.NamedType{Name: "String", NonNull: true},
									Directives:          nil,
								},
								{
									Description:         "The home planet of the human, or null if unknown",
									Name:                "homePlanet",
									ArgumentsDefinition: nil,
									Type:                document.NamedType{Name: "String"},
									Directives:          nil,
								},
								{
									Description: "Height in the preferred unit, default is meters",
									Name:        "height",
									ArgumentsDefinition: document.ArgumentsDefinition{
										{
											Name: "unit",
											Type: document.NamedType{Name: "LengthUnit"},
											DefaultValue: document.Value{
												ValueType: document.ValueTypeEnum,
												EnumValue: "METER"},
										},
									},
									Type: document.NamedType{Name: "Float"},
								},
								{
									Description:         "Mass in kilograms, or null if unknown",
									Name:                "mass",
									ArgumentsDefinition: nil,
									Type:                document.NamedType{Name: "Float"},
									Directives:          nil,
								},
								{
									Description:         "This human's friends, or an empty list if they have none",
									Name:                "friends",
									ArgumentsDefinition: nil,
									Type:                document.ListType{Type: document.NamedType{Name: "Character"}},
									Directives:          nil,
								},
								{
									Description: "The friends of the human exposed as a connection with edges",
									Name:        "friendsConnection",
									ArgumentsDefinition: document.ArgumentsDefinition{
										{
											Name: "first",
											Type: document.NamedType{Name: "Int"},
										},
										{
											Name: "after",
											Type: document.NamedType{Name: "ID"},
										},
									},
									Type: document.NamedType{
										Name:    "FriendsConnection",
										NonNull: true,
									},
								},
								{
									Description:         "The movies this human appears in",
									Name:                "appearsIn",
									ArgumentsDefinition: nil,
									Type: document.ListType{Type: document.NamedType{
										Name: "Episode",
									},
										NonNull: true,
									},
								},
								{
									Description:         "A list of starships this person has piloted, or an empty list if none",
									Name:                "starships",
									ArgumentsDefinition: nil,
									Type: document.ListType{Type: document.NamedType{
										Name: "Starship",
									},
									},
								},
							},
							ImplementsInterfaces: document.ImplementsInterfaces{"Character"},
							Directives:           nil,
						},
						{
							Description: "An autonomous mechanical character in the Star Wars universe",
							Name:        "Droid",
							FieldsDefinition: document.FieldDefinitions{
								{
									Description:         "The ID of the droid",
									Name:                "id",
									ArgumentsDefinition: nil,
									Type: document.NamedType{
										Name:    "ID",
										NonNull: true,
									},
								},
								{
									Description:         "What others call this droid",
									Name:                "name",
									ArgumentsDefinition: nil,
									Type: document.NamedType{
										Name:    "String",
										NonNull: true,
									},
								},
								{
									Description:         "This droid's friends, or an empty list if they have none",
									Name:                "friends",
									ArgumentsDefinition: nil,
									Type: document.ListType{Type: document.NamedType{
										Name: "Character",
									}},
								},
								{
									Description: "The friends of the droid exposed as a connection with edges",
									Name:        "friendsConnection",
									ArgumentsDefinition: document.ArgumentsDefinition{
										{
											Name: "first",
											Type: document.NamedType{
												Name: "Int",
											},
										},
										{
											Name: "after",
											Type: document.NamedType{
												Name: "ID",
											},
										},
									},
									Type: document.NamedType{
										Name:    "FriendsConnection",
										NonNull: true,
									},
								},
								{
									Description:         "The movies this droid appears in",
									Name:                "appearsIn",
									ArgumentsDefinition: nil,
									Type: document.ListType{Type: document.NamedType{
										Name: "Episode",
									},
										NonNull: true,
									},
								},
								{
									Description:         "This droid's primary function",
									Name:                "primaryFunction",
									ArgumentsDefinition: nil,
									Type: document.NamedType{
										Name: "String",
									},
								},
							},
							ImplementsInterfaces: document.ImplementsInterfaces{"Character"},
							Directives:           nil,
						},
						{
							Description: "A connection object for a character's friends",
							Name:        "FriendsConnection",
							FieldsDefinition: document.FieldDefinitions{
								{
									Description:         "The total number of friends",
									Name:                "totalCount",
									ArgumentsDefinition: nil,
									Type: document.NamedType{
										Name: "Int",
									},
								},
								{
									Description:         "The edges for each of the character's friends.",
									Name:                "edges",
									ArgumentsDefinition: nil,
									Type: document.ListType{Type: document.NamedType{
										Name: "FriendsEdge",
									}},
								},
								{
									Description:         "A list of the friends, as a convenience when edges are not needed.",
									Name:                "friends",
									ArgumentsDefinition: nil,
									Type: document.ListType{Type: document.NamedType{
										Name: "Character",
									}},
								},
								{
									Description:         "Information for paginating this connection",
									Name:                "pageInfo",
									ArgumentsDefinition: nil,
									Type: document.NamedType{
										Name:    "PageInfo",
										NonNull: true,
									},
								},
							},
							ImplementsInterfaces: nil,
							Directives:           nil,
						},
						{
							Description: "An edge object for a character's friends",
							Name:        "FriendsEdge",
							FieldsDefinition: document.FieldDefinitions{
								{
									Description:         "A cursor used for pagination",
									Name:                "cursor",
									ArgumentsDefinition: nil,
									Type: document.NamedType{
										Name:    "ID",
										NonNull: true,
									},
								},
								{
									Description:         "The character represented by this friendship edge",
									Name:                "node",
									ArgumentsDefinition: nil,
									Type: document.NamedType{
										Name: "Character",
									},
								},
							},
							ImplementsInterfaces: nil,
							Directives:           nil,
						},
						{
							Description: "Information for paginating this connection",
							Name:        "PageInfo",
							FieldsDefinition: document.FieldDefinitions{
								{
									Name:                "startCursor",
									ArgumentsDefinition: nil,
									Type: document.NamedType{
										Name: "ID",
									},
								},
								{
									Name:                "endCursor",
									ArgumentsDefinition: nil,
									Type: document.NamedType{
										Name: "ID",
									},
								},
								{
									Name:                "hasNextPage",
									ArgumentsDefinition: nil,
									Type: document.NamedType{
										Name:    "Boolean",
										NonNull: true,
									},
								},
							},
							ImplementsInterfaces: nil,
							Directives:           nil,
						},
						{
							Description: "Represents a review for a movie",
							Name:        "Review",
							FieldsDefinition: document.FieldDefinitions{
								{
									Description:         "The movie",
									Name:                "episode",
									ArgumentsDefinition: nil,
									Type: document.NamedType{
										Name: "Episode",
									},
								},
								{
									Description:         "The number of stars this review gave, 1-5",
									Name:                "stars",
									ArgumentsDefinition: nil,
									Type: document.NamedType{
										Name:    "Int",
										NonNull: true,
									},
								},
								{
									Description:         "Comment about the movie",
									Name:                "commentary",
									ArgumentsDefinition: nil,
									Type: document.NamedType{
										Name: "String",
									},
								},
							},
							ImplementsInterfaces: nil,
							Directives:           nil,
						},
						{
							Name: "Starship",
							FieldsDefinition: document.FieldDefinitions{
								{
									Description:         "The ID of the starship",
									Name:                "id",
									ArgumentsDefinition: nil,
									Type: document.NamedType{
										Name:    "ID",
										NonNull: true,
									},
								},
								{
									Description:         "The name of the starship",
									Name:                "name",
									ArgumentsDefinition: nil,
									Type: document.NamedType{
										Name:    "String",
										NonNull: true,
									},
								},
								{
									Description: "Length of the starship, along the longest axis",
									Name:        "length",
									ArgumentsDefinition: document.ArgumentsDefinition{
										{
											Name: "unit",
											Type: document.NamedType{
												Name: "LengthUnit",
											},
											DefaultValue: document.Value{
												ValueType: document.ValueTypeEnum,
												EnumValue: "METER",
											},
										},
									},
									Type: document.NamedType{
										Name: "Float",
									},
								},
								{
									Name: "coordinates",
									Type: document.ListType{
										Type: document.ListType{
											Type: document.NamedType{
												Name:    "Float",
												NonNull: true,
											},
											NonNull: true,
										},
									},
								},
							},
							ImplementsInterfaces: nil,
							Directives:           nil,
						},
					},
					InterfaceTypeDefinitions: []document.InterfaceTypeDefinition{
						{
							Description: "A character from the Star Wars universe",
							Name:        "Character",
							FieldsDefinition: document.FieldDefinitions{
								{
									Description:         "The ID of the character",
									Name:                "id",
									ArgumentsDefinition: nil,
									Type:                document.NamedType{Name: "ID", NonNull: true},
									Directives:          nil,
								},
								{
									Description:         "The name of the character",
									Name:                "name",
									ArgumentsDefinition: nil,
									Type:                document.NamedType{Name: "String", NonNull: true},
									Directives:          nil,
								},
								{
									Description:         "The friends of the character, or an empty list if they have none",
									Name:                "friends",
									ArgumentsDefinition: nil,
									Type:                document.ListType{Type: document.NamedType{Name: "Character"}},
									Directives:          nil,
								},
								{
									Description: "The friends of the character exposed as a connection with edges",
									Name:        "friendsConnection",
									ArgumentsDefinition: document.ArgumentsDefinition{
										{
											Name: "first",
											Type: document.NamedType{Name: "Int"},
										},
										{
											Name: "after",
											Type: document.NamedType{Name: "ID"},
										},
									},
									Type: document.NamedType{
										Name:    "FriendsConnection",
										NonNull: true,
									},
								},
								{
									Description:         "The movies this character appears in",
									Name:                "appearsIn",
									ArgumentsDefinition: nil,
									Type:                document.ListType{Type: document.NamedType{Name: "Episode"}, NonNull: true},
									Directives:          nil,
								},
							},
						},
					},
					UnionTypeDefinitions: []document.UnionTypeDefinition{
						{
							Name:             "SearchResult",
							UnionMemberTypes: document.UnionMemberTypes{"Human", "Droid", "Starship"},
							Directives:       nil,
						},
					},
					EnumTypeDefinitions: []document.EnumTypeDefinition{
						{
							Description: "The episodes in the Star Wars trilogy",
							Name:        "Episode",
							EnumValuesDefinition: document.EnumValueDefinitions{
								{
									Description: "Star Wars Episode IV: A New Hope, released in 1977.",
									EnumValue:   "NEWHOPE",
								},
								{
									Description: "Star Wars Episode V: The Empire Strikes Back, released in 1980.",
									EnumValue:   "EMPIRE",
								},
								{
									Description: "Star Wars Episode VI: Return of the Jedi, released in 1983.",
									EnumValue:   "JEDI",
								},
							},
						},
						{
							Description: "Units of height",
							Name:        "LengthUnit",
							EnumValuesDefinition: document.EnumValueDefinitions{
								{
									Description: "The standard unit around the world",
									EnumValue:   "METER",
								},
								{
									Description: "Primarily used in the United States",
									EnumValue:   "FOOT",
								},
							},
						},
					},
					InputObjectTypeDefinitions: document.InputObjectTypeDefinitions{
						{
							Description: "The input object sent when someone is creating a new review",
							Name:        "ReviewInput",
							InputFieldsDefinition: document.InputValueDefinitions{
								{
									Description: "0-5 stars",
									Name:        "stars",
									Type: document.NamedType{
										Name:    "Int",
										NonNull: true,
									},
								},
								{
									Description: "Comment about the movie, optional",
									Name:        "commentary",
									Type: document.NamedType{
										Name: "String",
									},
								},
								{
									Description: "Favorite color, optional",
									Name:        "favorite_color",
									Type: document.NamedType{
										Name: "ColorInput",
									},
								},
							},
						},
						{
							Description: "The input object sent when passing in a color",
							Name:        "ColorInput",
							InputFieldsDefinition: document.InputValueDefinitions{
								{
									Name: "red",
									Type: document.NamedType{
										Name:    "Int",
										NonNull: true,
									},
								},
								{
									Name: "green",
									Type: document.NamedType{
										Name:    "Int",
										NonNull: true,
									},
								},
								{
									Name: "blue",
									Type: document.NamedType{
										Name:    "Int",
										NonNull: true,
									},
								},
							},
						},
					},
				}))
				Expect(err).To(BeNil())
			})
		})
	}
*/
