package parser

import (
	"bytes"
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestTypeSystemDefinition(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parseTypeSystemDefinition", func() {
		tests := []struct {
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
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
				expectValues: Equal(document.TypeSystemDefinition{
					SchemaDefinition: document.SchemaDefinition{
						Query:    "Query",
						Mutation: "Mutation",
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
				expectValues: Equal(document.TypeSystemDefinition{
					ScalarTypeDefinitions: []document.ScalarTypeDefinition{
						{
							Name:        "JSON",
							Description: "this is a scalar",
						},
						{
							Name: "testName",
							Directives: document.Directives{
								document.Directive{
									Name: "fromTop",
									Arguments: document.Arguments{
										document.Argument{
											Name: "to",
											Value: document.StringValue{
												Val: "bottom",
											},
										},
									},
								},
							},
						},
						{
							Name:        "XML",
							Description: "this is another scalar",
						},
					}}),
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
				expectValues: Equal(document.TypeSystemDefinition{
					ObjectTypeDefinitions: []document.ObjectTypeDefinition{
						{
							Name:        "Person",
							Description: "this is a Person",
							FieldsDefinition: document.FieldsDefinition{
								document.FieldDefinition{
									Name: "name",
									Type: document.NamedType{
										Name: "String",
									},
								},
							},
						},
						{
							Name: "testType",
						},
						{
							Name:        "secondType",
							Description: "second Type",
						},
						{
							Name: "thirdType",
							Directives: document.Directives{
								document.Directive{
									Name: "fromTop",
									Arguments: document.Arguments{
										document.Argument{
											Name: "to",
											Value: document.StringValue{
												Val: "bottom",
											},
										},
									},
								},
							},
						},
						{
							Name:        "Animal",
							Description: "this is an Animal",
							FieldsDefinition: document.FieldsDefinition{
								document.FieldDefinition{
									Name: "age",
									Type: document.NamedType{
										Name: "Int",
									},
								},
							},
						},
					}}),
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
				expectValues: Equal(document.TypeSystemDefinition{
					InterfaceTypeDefinitions: []document.InterfaceTypeDefinition{
						{
							Name:        "firstEntity",
							Description: "describes firstEntity",
							FieldsDefinition: document.FieldsDefinition{
								document.FieldDefinition{
									Name: "name",
									Type: document.NamedType{
										Name: "String",
									},
								},
							},
						},
						{
							Name: "firstInterface",
						},
						{
							Name:        "secondInterface",
							Description: "second interface",
						},
						{
							Name: "thirdInterface",
							Directives: document.Directives{
								document.Directive{
									Name: "fromTop",
									Arguments: document.Arguments{
										document.Argument{
											Name: "to",
											Value: document.StringValue{
												Val: "bottom",
											},
										},
									},
								},
							},
						},
						{
							Name:        "secondEntity",
							Description: "describes secondEntity",
							FieldsDefinition: document.FieldsDefinition{
								document.FieldDefinition{
									Name: "age",
									Type: document.NamedType{
										Name: "Int",
									},
								},
							},
						},
					},
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
				expectValues: Equal(document.TypeSystemDefinition{
					UnionTypeDefinitions: []document.UnionTypeDefinition{
						{
							Name:        "SearchResult",
							Description: "unifies SearchResult",
							UnionMemberTypes: document.UnionMemberTypes{
								"Photo", "Person",
							},
						},
						{
							Name: "thirdUnion",
						},
						{
							Name:        "secondUnion",
							Description: "second union",
						},
						{
							Name: "firstUnion",
							Directives: document.Directives{
								document.Directive{
									Name: "fromTop",
									Arguments: document.Arguments{
										document.Argument{
											Name: "to",
											Value: document.StringValue{
												Val: "bottom",
											},
										},
									},
								},
							},
						},
						{
							Name:        "UnionExample",
							Description: "unifies UnionExample",
							UnionMemberTypes: document.UnionMemberTypes{
								"First", "Second",
							},
						},
					}}),
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
				expectValues: Equal(document.TypeSystemDefinition{
					EnumTypeDefinitions: []document.EnumTypeDefinition{
						{
							Name:        "Direction",
							Description: "describes direction",
							EnumValuesDefinition: document.EnumValuesDefinition{
								{
									EnumValue: "NORTH",
								},
							},
						},
						{
							Name: "thirdEnum",
						},
						{
							Name:        "secondEnum",
							Description: "second enum",
						},
						{
							Name: "firstEnum",
							Directives: document.Directives{
								document.Directive{
									Name: "fromTop",
									Arguments: document.Arguments{
										document.Argument{
											Name: "to",
											Value: document.StringValue{
												Val: "bottom",
											},
										},
									},
								},
							},
						},
						{
							Name:        "EnumExample",
							Description: "enumerates EnumExample",
							EnumValuesDefinition: document.EnumValuesDefinition{
								{
									EnumValue: "NORTH",
								},
							},
						},
					}}),
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
				expectValues: Equal(document.TypeSystemDefinition{
					InputObjectTypeDefinitions: []document.InputObjectTypeDefinition{
						{
							Name:        "Person",
							Description: "describes Person",
							InputFieldsDefinition: document.InputFieldsDefinition{
								document.InputValueDefinition{
									Name: "name",
									Type: document.NamedType{
										Name: "String",
									},
								},
							},
						},
						{
							Name: "thirdInput",
						},
						{
							Name:        "secondInput",
							Description: "second input",
						},
						{
							Name: "firstInput",
							Directives: document.Directives{
								document.Directive{
									Name: "fromTop",
									Arguments: document.Arguments{
										document.Argument{
											Name: "to",
											Value: document.StringValue{
												Val: "bottom",
											},
										},
									},
								},
							},
						},
						{
							Name:        "InputExample",
							Description: "inputs InputExample",
							InputFieldsDefinition: document.InputFieldsDefinition{
								document.InputValueDefinition{
									Name: "name",
									Type: document.NamedType{
										Name: "String",
									},
								},
							},
						},
					}}),
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
				expectValues: Equal(document.TypeSystemDefinition{
					DirectiveDefinitions: []document.DirectiveDefinition{
						{
							Name:        "somewhere",
							Description: "describes somewhere",
							DirectiveLocations: document.DirectiveLocations{
								document.DirectiveLocationQUERY,
							},
						},
						{
							Name: "somehow",
							DirectiveLocations: document.DirectiveLocations{
								document.DirectiveLocationMUTATION,
							},
						},
						{
							Name:        "someway",
							Description: "describes someway",
							DirectiveLocations: document.DirectiveLocations{
								document.DirectiveLocationSUBSCRIPTION,
							},
						},
					}}),
			},
			{
				it: "should not parse TypeSystemDefinition when type has no valid TypeSystemDefinition identifier",
				input: `
				"describes nonsense"
				nonsense @ somewhere on QUERY
				`,
				expectErr:    Not(BeNil()),
				expectValues: Equal(document.TypeSystemDefinition{}),
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
				expectValues: Equal(document.TypeSystemDefinition{
					SchemaDefinition: document.SchemaDefinition{
						Query:    "Query",
						Mutation: "Mutation",
					}}),
			},
		}
		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				reader := bytes.NewReader([]byte(test.input))
				parser := NewParser()
				parser.l.SetInput(reader)

				val, err := parser.parseTypeSystemDefinition()
				Expect(val).To(test.expectValues)
				Expect(err).To(test.expectErr)
			})
		}
	})

	starWarsSchema := []byte(`
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
	`)

	g.Describe("StarWars Schema", func() {
		g.It("should parse", func() {
			reader := bytes.NewReader(starWarsSchema)
			parser := NewParser()
			parser.l.SetInput(reader)

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
								Name: "__schema",
								Type: document.NamedType{
									Name:    "__Schema",
									NonNull: true,
								},
							},
							{
								Name: "__type",
								Type: document.NamedType{
									Name:    "__Type",
									NonNull: true,
								},
							},
							{
								Name: "hero",
								ArgumentsDefinition: document.ArgumentsDefinition{
									{
										Description: "",
										Name:        "episode",
										Type:        document.NamedType{Name: "Episode"},
									},
								},
								Type: document.NamedType{Name: "Character"},
							},
							{
								Description: "",
								Name:        "reviews",
								ArgumentsDefinition: document.ArgumentsDefinition{
									{
										Description: "",
										Name:        "episode",
										Type:        document.NamedType{Name: "Episode", NonNull: true},
									},
								},
								Type: document.ListType{Type: document.NamedType{Name: "Review"}},
							},
							{
								Description: "",
								Name:        "search",
								ArgumentsDefinition: document.ArgumentsDefinition{
									{
										Description: "",
										Name:        "text",
										Type:        document.NamedType{Name: "String"},
									},
								},
								Type: document.ListType{Type: document.NamedType{Name: "SearchResult"}},
							},
							{
								Description: "",
								Name:        "character",
								ArgumentsDefinition: document.ArgumentsDefinition{
									{
										Description: "",
										Name:        "id",
										Type:        document.NamedType{Name: "ID", NonNull: true},
									},
								},
								Type: document.NamedType{Name: "Character"},
							},
							{
								Description: "",
								Name:        "droid",
								ArgumentsDefinition: document.ArgumentsDefinition{
									{
										Description: "",
										Name:        "id",
										Type:        document.NamedType{Name: "ID", NonNull: true},
									},
								},
								Type: document.NamedType{Name: "Droid"},
							},
							{
								Description: "",
								Name:        "human",
								ArgumentsDefinition: document.ArgumentsDefinition{
									{
										Description: "",
										Name:        "id",
										Type:        document.NamedType{Name: "ID", NonNull: true},
									},
								},
								Type: document.NamedType{Name: "Human"},
							},
							{
								Description: "",
								Name:        "starship",
								ArgumentsDefinition: document.ArgumentsDefinition{
									{
										Description: "",
										Name:        "id",
										Type:        document.NamedType{Name: "ID", NonNull: true},
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
						FieldsDefinition: document.FieldsDefinition{
							{
								Description: "",
								Name:        "createReview",
								ArgumentsDefinition: document.ArgumentsDefinition{
									{
										Description: "",
										Name:        "episode",
										Type:        document.NamedType{Name: "Episode"},
									},
									{
										Description: "",
										Name:        "review",
										Type:        document.NamedType{Name: "ReviewInput", NonNull: true},
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
						FieldsDefinition: document.FieldsDefinition{
							{
								Description: "",
								Name:        "reviewAdded",
								ArgumentsDefinition: document.ArgumentsDefinition{
									{
										Description: "",
										Name:        "episode",
										Type:        document.NamedType{Name: "Episode"},
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
						FieldsDefinition: document.FieldsDefinition{
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
										Description:  "",
										Name:         "unit",
										Type:         document.NamedType{Name: "LengthUnit"},
										DefaultValue: document.EnumValue{Name: "METER"},
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
										Description: "",
										Name:        "first",
										Type:        document.NamedType{Name: "Int"},
									},
									{
										Description: "",
										Name:        "after",
										Type:        document.NamedType{Name: "ID"},
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
						FieldsDefinition: document.FieldsDefinition{
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
										Description: "",
										Name:        "first",
										Type: document.NamedType{
											Name: "Int",
										},
									},
									{
										Description: "",
										Name:        "after",
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
						FieldsDefinition: document.FieldsDefinition{
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
						FieldsDefinition: document.FieldsDefinition{
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
						FieldsDefinition: document.FieldsDefinition{
							{
								Description:         "",
								Name:                "startCursor",
								ArgumentsDefinition: nil,
								Type: document.NamedType{
									Name: "ID",
								},
							},
							{
								Description:         "",
								Name:                "endCursor",
								ArgumentsDefinition: nil,
								Type: document.NamedType{
									Name: "ID",
								},
							},
							{
								Description:         "",
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
						FieldsDefinition: document.FieldsDefinition{
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
						Description: "",
						Name:        "Starship",
						FieldsDefinition: document.FieldsDefinition{
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
										Description: "",
										Name:        "unit",
										Type: document.NamedType{
											Name: "LengthUnit",
										},
										DefaultValue: document.EnumValue{Name: "METER"},
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
						FieldsDefinition: document.FieldsDefinition{
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
										Description: "",
										Name:        "first",
										Type:        document.NamedType{Name: "Int"},
									},
									{
										Description: "",
										Name:        "after",
										Type:        document.NamedType{Name: "ID"},
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
						Description:      "",
						Name:             "SearchResult",
						UnionMemberTypes: document.UnionMemberTypes{"Human", "Droid", "Starship"},
						Directives:       nil,
					},
				},
				EnumTypeDefinitions: []document.EnumTypeDefinition{
					{
						Description: "The episodes in the Star Wars trilogy",
						Name:        "Episode",
						EnumValuesDefinition: document.EnumValuesDefinition{
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
						EnumValuesDefinition: document.EnumValuesDefinition{
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
						InputFieldsDefinition: document.InputFieldsDefinition{
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
						InputFieldsDefinition: document.InputFieldsDefinition{
							{
								Description: "",
								Name:        "red",
								Type: document.NamedType{
									Name:    "Int",
									NonNull: true,
								},
							},
							{
								Description: "",
								Name:        "green",
								Type: document.NamedType{
									Name:    "Int",
									NonNull: true,
								},
							},
							{
								Description: "",
								Name:        "blue",
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
