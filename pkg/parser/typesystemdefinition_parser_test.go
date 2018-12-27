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
						Query:    []byte("Query"),
						Mutation: []byte("Mutation"),
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
							Name:        []byte("JSON"),
							Description: []byte("this is a scalar"),
						},
						{
							Name: []byte("testName"),
							Directives: document.Directives{
								document.Directive{
									Name: []byte("fromTop"),
									Arguments: document.Arguments{
										document.Argument{
											Name: []byte("to"),
											Value: document.StringValue{
												Val: []byte("bottom"),
											},
										},
									},
								},
							},
						},
						{
							Name:        []byte("XML"),
							Description: []byte("this is another scalar"),
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
							Name:        []byte("Person"),
							Description: []byte("this is a Person"),
							FieldsDefinition: document.FieldsDefinition{
								document.FieldDefinition{
									Name: []byte("name"),
									Type: document.NamedType{
										Name: []byte("String"),
									},
								},
							},
						},
						{
							Name: []byte("testType"),
						},
						{
							Name:        []byte("secondType"),
							Description: []byte("second Type"),
						},
						{
							Name: []byte("thirdType"),
							Directives: document.Directives{
								document.Directive{
									Name: []byte("fromTop"),
									Arguments: document.Arguments{
										document.Argument{
											Name: []byte("to"),
											Value: document.StringValue{
												Val: []byte("bottom"),
											},
										},
									},
								},
							},
						},
						{
							Name:        []byte("Animal"),
							Description: []byte("this is an Animal"),
							FieldsDefinition: document.FieldsDefinition{
								document.FieldDefinition{
									Name: []byte("age"),
									Type: document.NamedType{
										Name: []byte("Int"),
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
							Name:        []byte("firstEntity"),
							Description: []byte("describes firstEntity"),
							FieldsDefinition: document.FieldsDefinition{
								document.FieldDefinition{
									Name: []byte("name"),
									Type: document.NamedType{
										Name: []byte("String"),
									},
								},
							},
						},
						{
							Name: []byte("firstInterface"),
						},
						{
							Name:        []byte("secondInterface"),
							Description: []byte("second interface"),
						},
						{
							Name: []byte("thirdInterface"),
							Directives: document.Directives{
								document.Directive{
									Name: []byte("fromTop"),
									Arguments: document.Arguments{
										document.Argument{
											Name: []byte("to"),
											Value: document.StringValue{
												Val: []byte("bottom"),
											},
										},
									},
								},
							},
						},
						{
							Name:        []byte("secondEntity"),
							Description: []byte("describes secondEntity"),
							FieldsDefinition: document.FieldsDefinition{
								document.FieldDefinition{
									Name: []byte("age"),
									Type: document.NamedType{
										Name: []byte("Int"),
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
							Name:        []byte("SearchResult"),
							Description: []byte("unifies SearchResult"),
							UnionMemberTypes: document.UnionMemberTypes{
								[]byte("Photo"),
								[]byte("Person"),
							},
						},
						{
							Name: []byte("thirdUnion"),
						},
						{
							Name:        []byte("secondUnion"),
							Description: []byte("second union"),
						},
						{
							Name: []byte("firstUnion"),
							Directives: document.Directives{
								document.Directive{
									Name: []byte("fromTop"),
									Arguments: document.Arguments{
										document.Argument{
											Name: []byte("to"),
											Value: document.StringValue{
												Val: []byte("bottom"),
											},
										},
									},
								},
							},
						},
						{
							Name:        []byte("UnionExample"),
							Description: []byte("unifies UnionExample"),
							UnionMemberTypes: document.UnionMemberTypes{
								[]byte("First"),
								[]byte("Second"),
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
							Name:        []byte("Direction"),
							Description: []byte("describes direction"),
							EnumValuesDefinition: document.EnumValuesDefinition{
								{
									EnumValue: []byte("NORTH"),
								},
							},
						},
						{
							Name: []byte("thirdEnum"),
						},
						{
							Name:        []byte("secondEnum"),
							Description: []byte("second enum"),
						},
						{
							Name: []byte("firstEnum"),
							Directives: document.Directives{
								document.Directive{
									Name: []byte("fromTop"),
									Arguments: document.Arguments{
										document.Argument{
											Name: []byte("to"),
											Value: document.StringValue{
												Val: []byte("bottom"),
											},
										},
									},
								},
							},
						},
						{
							Name:        []byte("EnumExample"),
							Description: []byte("enumerates EnumExample"),
							EnumValuesDefinition: document.EnumValuesDefinition{
								{
									EnumValue: []byte("NORTH"),
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
							Name:        []byte("Person"),
							Description: []byte("describes Person"),
							InputFieldsDefinition: document.InputFieldsDefinition{
								document.InputValueDefinition{
									Name: []byte("name"),
									Type: document.NamedType{
										Name: []byte("String"),
									},
								},
							},
						},
						{
							Name: []byte("thirdInput"),
						},
						{
							Name:        []byte("secondInput"),
							Description: []byte("second input"),
						},
						{
							Name: []byte("firstInput"),
							Directives: document.Directives{
								document.Directive{
									Name: []byte("fromTop"),
									Arguments: document.Arguments{
										document.Argument{
											Name: []byte("to"),
											Value: document.StringValue{
												Val: []byte("bottom"),
											},
										},
									},
								},
							},
						},
						{
							Name:        []byte("InputExample"),
							Description: []byte("inputs InputExample"),
							InputFieldsDefinition: document.InputFieldsDefinition{
								document.InputValueDefinition{
									Name: []byte("name"),
									Type: document.NamedType{
										Name: []byte("String"),
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
							Name:        []byte("somewhere"),
							Description: []byte("describes somewhere"),
							DirectiveLocations: document.DirectiveLocations{
								document.DirectiveLocationQUERY,
							},
						},
						{
							Name: []byte("somehow"),
							DirectiveLocations: document.DirectiveLocations{
								document.DirectiveLocationMUTATION,
							},
						},
						{
							Name:        []byte("someway"),
							Description: []byte("describes someway"),
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
						Query:    []byte("Query"),
						Mutation: []byte("Mutation"),
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
					Query:        []byte("Query"),
					Mutation:     []byte("Mutation"),
					Subscription: []byte("Subscription"),
				},
				ScalarTypeDefinitions: nil,
				ObjectTypeDefinitions: []document.ObjectTypeDefinition{
					{
						Description: []byte("The query type, represents all of the entry points into our object graph"),
						Name:        []byte("Query"),
						FieldsDefinition: []document.FieldDefinition{
							{
								Name: []byte("__schema"),
								Type: document.NamedType{
									Name:    []byte("__Schema"),
									NonNull: true,
								},
							},
							{
								Name: []byte("__type"),
								Type: document.NamedType{
									Name:    []byte("__Type"),
									NonNull: false,
								},
								ArgumentsDefinition: []document.InputValueDefinition{
									{
										Name: []byte("name"),
										Type: document.NamedType{
											Name:    []byte("String"),
											NonNull: true,
										},
									},
								},
							},
							{
								Name: []byte("hero"),
								ArgumentsDefinition: document.ArgumentsDefinition{
									{
										Name: []byte("episode"),
										Type: document.NamedType{Name: []byte("Episode")},
									},
								},
								Type: document.NamedType{Name: []byte("Character")},
							},
							{
								Name: []byte("reviews"),
								ArgumentsDefinition: document.ArgumentsDefinition{
									{
										Name: []byte("episode"),
										Type: document.NamedType{Name: []byte("Episode"), NonNull: true},
									},
								},
								Type: document.ListType{Type: document.NamedType{Name: []byte("Review")}},
							},
							{
								Name: []byte("search"),
								ArgumentsDefinition: document.ArgumentsDefinition{
									{
										Name: []byte("text"),
										Type: document.NamedType{Name: []byte("String")},
									},
								},
								Type: document.ListType{Type: document.NamedType{Name: []byte("SearchResult")}},
							},
							{
								Name: []byte("character"),
								ArgumentsDefinition: document.ArgumentsDefinition{
									{
										Name: []byte("id"),
										Type: document.NamedType{Name: []byte("ID"), NonNull: true},
									},
								},
								Type: document.NamedType{Name: []byte("Character")},
							},
							{
								Name: []byte("droid"),
								ArgumentsDefinition: document.ArgumentsDefinition{
									{
										Name: []byte("id"),
										Type: document.NamedType{Name: []byte("ID"), NonNull: true},
									},
								},
								Type: document.NamedType{Name: []byte("Droid")},
							},
							{
								Name: []byte("human"),
								ArgumentsDefinition: document.ArgumentsDefinition{
									{
										Name: []byte("id"),
										Type: document.NamedType{Name: []byte("ID"), NonNull: true},
									},
								},
								Type: document.NamedType{Name: []byte("Human")},
							},
							{
								Name: []byte("starship"),
								ArgumentsDefinition: document.ArgumentsDefinition{
									{
										Name: []byte("id"),
										Type: document.NamedType{Name: []byte("ID"), NonNull: true},
									},
								},
								Type: document.NamedType{Name: []byte("Starship")},
							},
						},
						ImplementsInterfaces: nil,
						Directives:           nil,
					},
					{
						Description: []byte("The mutation type, represents all updates we can make to our data"),
						Name:        []byte("Mutation"),
						FieldsDefinition: document.FieldsDefinition{
							{
								Name: []byte("createReview"),
								ArgumentsDefinition: document.ArgumentsDefinition{
									{
										Name: []byte("episode"),
										Type: document.NamedType{Name: []byte("Episode")},
									},
									{
										Name: []byte("review"),
										Type: document.NamedType{Name: []byte("ReviewInput"), NonNull: true},
									},
								},
								Type: document.NamedType{Name: []byte("Review")},
							},
						},
						ImplementsInterfaces: nil,
						Directives:           nil,
					},
					{
						Description: []byte("The subscription type, represents all subscriptions we can make to our data"),
						Name:        []byte("Subscription"),
						FieldsDefinition: document.FieldsDefinition{
							{
								Name: []byte("reviewAdded"),
								ArgumentsDefinition: document.ArgumentsDefinition{
									{
										Name: []byte("episode"),
										Type: document.NamedType{Name: []byte("Episode")},
									},
								},
								Type: document.NamedType{Name: []byte("Review")},
							},
						},
						ImplementsInterfaces: nil,
						Directives:           nil,
					},
					{
						Description: []byte("A humanoid creature from the Star Wars universe"),
						Name:        []byte("Human"),
						FieldsDefinition: document.FieldsDefinition{
							{
								Description:         []byte("The ID of the human"),
								Name:                []byte("id"),
								ArgumentsDefinition: nil,
								Type:                document.NamedType{Name: []byte("ID"), NonNull: true},
								Directives:          nil,
							},
							{
								Description:         []byte("What this human calls themselves"),
								Name:                []byte("name"),
								ArgumentsDefinition: nil,
								Type:                document.NamedType{Name: []byte("String"), NonNull: true},
								Directives:          nil,
							},
							{
								Description:         []byte("The home planet of the human, or null if unknown"),
								Name:                []byte("homePlanet"),
								ArgumentsDefinition: nil,
								Type:                document.NamedType{Name: []byte("String")},
								Directives:          nil,
							},
							{
								Description: []byte("Height in the preferred unit, default is meters"),
								Name:        []byte("height"),
								ArgumentsDefinition: document.ArgumentsDefinition{
									{
										Name:         []byte("unit"),
										Type:         document.NamedType{Name: []byte("LengthUnit")},
										DefaultValue: document.EnumValue{Name: []byte("METER")},
									},
								},
								Type: document.NamedType{Name: []byte("Float")},
							},
							{
								Description:         []byte("Mass in kilograms, or null if unknown"),
								Name:                []byte("mass"),
								ArgumentsDefinition: nil,
								Type:                document.NamedType{Name: []byte("Float")},
								Directives:          nil,
							},
							{
								Description:         []byte("This human's friends, or an empty list if they have none"),
								Name:                []byte("friends"),
								ArgumentsDefinition: nil,
								Type:                document.ListType{Type: document.NamedType{Name: []byte("Character")}},
								Directives:          nil,
							},
							{
								Description: []byte("The friends of the human exposed as a connection with edges"),
								Name:        []byte("friendsConnection"),
								ArgumentsDefinition: document.ArgumentsDefinition{
									{
										Name: []byte("first"),
										Type: document.NamedType{Name: []byte("Int")},
									},
									{
										Name: []byte("after"),
										Type: document.NamedType{Name: []byte("ID")},
									},
								},
								Type: document.NamedType{
									Name:    []byte("FriendsConnection"),
									NonNull: true,
								},
							},
							{
								Description:         []byte("The movies this human appears in"),
								Name:                []byte("appearsIn"),
								ArgumentsDefinition: nil,
								Type: document.ListType{Type: document.NamedType{
									Name: []byte("Episode"),
								},
									NonNull: true,
								},
							},
							{
								Description:         []byte("A list of starships this person has piloted, or an empty list if none"),
								Name:                []byte("starships"),
								ArgumentsDefinition: nil,
								Type: document.ListType{Type: document.NamedType{
									Name: []byte("Starship"),
								},
								},
							},
						},
						ImplementsInterfaces: document.ImplementsInterfaces{[]byte("Character")},
						Directives:           nil,
					},
					{
						Description: []byte("An autonomous mechanical character in the Star Wars universe"),
						Name:        []byte("Droid"),
						FieldsDefinition: document.FieldsDefinition{
							{
								Description:         []byte("The ID of the droid"),
								Name:                []byte("id"),
								ArgumentsDefinition: nil,
								Type: document.NamedType{
									Name:    []byte("ID"),
									NonNull: true,
								},
							},
							{
								Description:         []byte("What others call this droid"),
								Name:                []byte("name"),
								ArgumentsDefinition: nil,
								Type: document.NamedType{
									Name:    []byte("String"),
									NonNull: true,
								},
							},
							{
								Description:         []byte("This droid's friends, or an empty list if they have none"),
								Name:                []byte("friends"),
								ArgumentsDefinition: nil,
								Type: document.ListType{Type: document.NamedType{
									Name: []byte("Character"),
								}},
							},
							{
								Description: []byte("The friends of the droid exposed as a connection with edges"),
								Name:        []byte("friendsConnection"),
								ArgumentsDefinition: document.ArgumentsDefinition{
									{
										Name: []byte("first"),
										Type: document.NamedType{
											Name: []byte("Int"),
										},
									},
									{
										Name: []byte("after"),
										Type: document.NamedType{
											Name: []byte("ID"),
										},
									},
								},
								Type: document.NamedType{
									Name:    []byte("FriendsConnection"),
									NonNull: true,
								},
							},
							{
								Description:         []byte("The movies this droid appears in"),
								Name:                []byte("appearsIn"),
								ArgumentsDefinition: nil,
								Type: document.ListType{Type: document.NamedType{
									Name: []byte("Episode"),
								},
									NonNull: true,
								},
							},
							{
								Description:         []byte("This droid's primary function"),
								Name:                []byte("primaryFunction"),
								ArgumentsDefinition: nil,
								Type: document.NamedType{
									Name: []byte("String"),
								},
							},
						},
						ImplementsInterfaces: document.ImplementsInterfaces{[]byte("Character")},
						Directives:           nil,
					},
					{
						Description: []byte("A connection object for a character's friends"),
						Name:        []byte("FriendsConnection"),
						FieldsDefinition: document.FieldsDefinition{
							{
								Description:         []byte("The total number of friends"),
								Name:                []byte("totalCount"),
								ArgumentsDefinition: nil,
								Type: document.NamedType{
									Name: []byte("Int"),
								},
							},
							{
								Description:         []byte("The edges for each of the character's friends."),
								Name:                []byte("edges"),
								ArgumentsDefinition: nil,
								Type: document.ListType{Type: document.NamedType{
									Name: []byte("FriendsEdge"),
								}},
							},
							{
								Description:         []byte("A list of the friends, as a convenience when edges are not needed."),
								Name:                []byte("friends"),
								ArgumentsDefinition: nil,
								Type: document.ListType{Type: document.NamedType{
									Name: []byte("Character"),
								}},
							},
							{
								Description:         []byte("Information for paginating this connection"),
								Name:                []byte("pageInfo"),
								ArgumentsDefinition: nil,
								Type: document.NamedType{
									Name:    []byte("PageInfo"),
									NonNull: true,
								},
							},
						},
						ImplementsInterfaces: nil,
						Directives:           nil,
					},
					{
						Description: []byte("An edge object for a character's friends"),
						Name:        []byte("FriendsEdge"),
						FieldsDefinition: document.FieldsDefinition{
							{
								Description:         []byte("A cursor used for pagination"),
								Name:                []byte("cursor"),
								ArgumentsDefinition: nil,
								Type: document.NamedType{
									Name:    []byte("ID"),
									NonNull: true,
								},
							},
							{
								Description:         []byte("The character represented by this friendship edge"),
								Name:                []byte("node"),
								ArgumentsDefinition: nil,
								Type: document.NamedType{
									Name: []byte("Character"),
								},
							},
						},
						ImplementsInterfaces: nil,
						Directives:           nil,
					},
					{
						Description: []byte("Information for paginating this connection"),
						Name:        []byte("PageInfo"),
						FieldsDefinition: document.FieldsDefinition{
							{
								Name:                []byte("startCursor"),
								ArgumentsDefinition: nil,
								Type: document.NamedType{
									Name: []byte("ID"),
								},
							},
							{
								Name:                []byte("endCursor"),
								ArgumentsDefinition: nil,
								Type: document.NamedType{
									Name: []byte("ID"),
								},
							},
							{
								Name:                []byte("hasNextPage"),
								ArgumentsDefinition: nil,
								Type: document.NamedType{
									Name:    []byte("Boolean"),
									NonNull: true,
								},
							},
						},
						ImplementsInterfaces: nil,
						Directives:           nil,
					},
					{
						Description: []byte("Represents a review for a movie"),
						Name:        []byte("Review"),
						FieldsDefinition: document.FieldsDefinition{
							{
								Description:         []byte("The movie"),
								Name:                []byte("episode"),
								ArgumentsDefinition: nil,
								Type: document.NamedType{
									Name: []byte("Episode"),
								},
							},
							{
								Description:         []byte("The number of stars this review gave, 1-5"),
								Name:                []byte("stars"),
								ArgumentsDefinition: nil,
								Type: document.NamedType{
									Name:    []byte("Int"),
									NonNull: true,
								},
							},
							{
								Description:         []byte("Comment about the movie"),
								Name:                []byte("commentary"),
								ArgumentsDefinition: nil,
								Type: document.NamedType{
									Name: []byte("String"),
								},
							},
						},
						ImplementsInterfaces: nil,
						Directives:           nil,
					},
					{
						Name: []byte("Starship"),
						FieldsDefinition: document.FieldsDefinition{
							{
								Description:         []byte("The ID of the starship"),
								Name:                []byte("id"),
								ArgumentsDefinition: nil,
								Type: document.NamedType{
									Name:    []byte("ID"),
									NonNull: true,
								},
							},
							{
								Description:         []byte("The name of the starship"),
								Name:                []byte("name"),
								ArgumentsDefinition: nil,
								Type: document.NamedType{
									Name:    []byte("String"),
									NonNull: true,
								},
							},
							{
								Description: []byte("Length of the starship, along the longest axis"),
								Name:        []byte("length"),
								ArgumentsDefinition: document.ArgumentsDefinition{
									{
										Name: []byte("unit"),
										Type: document.NamedType{
											Name: []byte("LengthUnit"),
										},
										DefaultValue: document.EnumValue{Name: []byte("METER")},
									},
								},
								Type: document.NamedType{
									Name: []byte("Float"),
								},
							},
							{
								Name: []byte("coordinates"),
								Type: document.ListType{
									Type: document.ListType{
										Type: document.NamedType{
											Name:    []byte("Float"),
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
						Description: []byte("A character from the Star Wars universe"),
						Name:        []byte("Character"),
						FieldsDefinition: document.FieldsDefinition{
							{
								Description:         []byte("The ID of the character"),
								Name:                []byte("id"),
								ArgumentsDefinition: nil,
								Type:                document.NamedType{Name: []byte("ID"), NonNull: true},
								Directives:          nil,
							},
							{
								Description:         []byte("The name of the character"),
								Name:                []byte("name"),
								ArgumentsDefinition: nil,
								Type:                document.NamedType{Name: []byte("String"), NonNull: true},
								Directives:          nil,
							},
							{
								Description:         []byte("The friends of the character, or an empty list if they have none"),
								Name:                []byte("friends"),
								ArgumentsDefinition: nil,
								Type:                document.ListType{Type: document.NamedType{Name: []byte("Character")}},
								Directives:          nil,
							},
							{
								Description: []byte("The friends of the character exposed as a connection with edges"),
								Name:        []byte("friendsConnection"),
								ArgumentsDefinition: document.ArgumentsDefinition{
									{
										Name: []byte("first"),
										Type: document.NamedType{Name: []byte("Int")},
									},
									{
										Name: []byte("after"),
										Type: document.NamedType{Name: []byte("ID")},
									},
								},
								Type: document.NamedType{
									Name:    []byte("FriendsConnection"),
									NonNull: true,
								},
							},
							{
								Description:         []byte("The movies this character appears in"),
								Name:                []byte("appearsIn"),
								ArgumentsDefinition: nil,
								Type:                document.ListType{Type: document.NamedType{Name: []byte("Episode")}, NonNull: true},
								Directives:          nil,
							},
						},
					},
				},
				UnionTypeDefinitions: []document.UnionTypeDefinition{
					{
						Name:             []byte("SearchResult"),
						UnionMemberTypes: document.UnionMemberTypes{[]byte("Human"), []byte("Droid"), []byte("Starship")},
						Directives:       nil,
					},
				},
				EnumTypeDefinitions: []document.EnumTypeDefinition{
					{
						Description: []byte("The episodes in the Star Wars trilogy"),
						Name:        []byte("Episode"),
						EnumValuesDefinition: document.EnumValuesDefinition{
							{
								Description: []byte("Star Wars Episode IV: A New Hope, released in 1977."),
								EnumValue:   []byte("NEWHOPE"),
							},
							{
								Description: []byte("Star Wars Episode V: The Empire Strikes Back, released in 1980."),
								EnumValue:   []byte("EMPIRE"),
							},
							{
								Description: []byte("Star Wars Episode VI: Return of the Jedi, released in 1983."),
								EnumValue:   []byte("JEDI"),
							},
						},
					},
					{
						Description: []byte("Units of height"),
						Name:        []byte("LengthUnit"),
						EnumValuesDefinition: document.EnumValuesDefinition{
							{
								Description: []byte("The standard unit around the world"),
								EnumValue:   []byte("METER"),
							},
							{
								Description: []byte("Primarily used in the United States"),
								EnumValue:   []byte("FOOT"),
							},
						},
					},
				},
				InputObjectTypeDefinitions: document.InputObjectTypeDefinitions{
					{
						Description: []byte("The input object sent when someone is creating a new review"),
						Name:        []byte("ReviewInput"),
						InputFieldsDefinition: document.InputFieldsDefinition{
							{
								Description: []byte("0-5 stars"),
								Name:        []byte("stars"),
								Type: document.NamedType{
									Name:    []byte("Int"),
									NonNull: true,
								},
							},
							{
								Description: []byte("Comment about the movie, optional"),
								Name:        []byte("commentary"),
								Type: document.NamedType{
									Name: []byte("String"),
								},
							},
							{
								Description: []byte("Favorite color, optional"),
								Name:        []byte("favorite_color"),
								Type: document.NamedType{
									Name: []byte("ColorInput"),
								},
							},
						},
					},
					{
						Description: []byte("The input object sent when passing in a color"),
						Name:        []byte("ColorInput"),
						InputFieldsDefinition: document.InputFieldsDefinition{
							{
								Name: []byte("red"),
								Type: document.NamedType{
									Name:    []byte("Int"),
									NonNull: true,
								},
							},
							{
								Name: []byte("green"),
								Type: document.NamedType{
									Name:    []byte("Int"),
									NonNull: true,
								},
							},
							{
								Name: []byte("blue"),
								Type: document.NamedType{
									Name:    []byte("Int"),
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
