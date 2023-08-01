package federationdata

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

func sortNodesAndFields(nodes []plan.TypeField) {
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].TypeName < nodes[j].TypeName
	})
	for i := range nodes {
		sort.Strings(nodes[i].FieldNames)
	}
}

func TestLocalTypeFieldExtractor_GetAllNodes(t *testing.T) {
	run := func(t *testing.T, SDL string, expectedRoot, expectedChild []plan.TypeField) {
		t.Helper()

		document := unsafeparser.ParseGraphqlDocumentString(SDL)
		extractor := NewLocalTypeFieldExtractor(&document)
		gotRoot, gotChild := extractor.GetAllNodes()

		sortNodesAndFields(gotRoot)
		sortNodesAndFields(gotChild)

		assert.Equal(t, expectedRoot, gotRoot, "root nodes dont match")
		assert.Equal(t, expectedChild, gotChild, "child nodes dont match")
	}

	t.Run("only root operation", func(t *testing.T) {
		run(t, `
			extend type Query {
				me: User
			}

			extend type Mutation {
				addUser(id: ID!): User
				deleteUser(id: ID!): User
			}

			extend type Subscription {
				userChanges(id: ID!): User!
			}

			type User {
				id: ID!
			}
		`,
			[]plan.TypeField{
				{TypeName: "Mutation", FieldNames: []string{"addUser", "deleteUser"}},
				{TypeName: "Query", FieldNames: []string{"me"}},
				{TypeName: "Subscription", FieldNames: []string{"userChanges"}},
			},
			[]plan.TypeField{
				{TypeName: "User", FieldNames: []string{"id"}},
			})
	})
	t.Run("orphan pair", func(t *testing.T) {
		run(t, `
			extend type Query {
				me: User
			}

			type User {
				id: ID!
			}

			# This type isn't connected to a root node, so
			# it doesn't show up as a child node.
			type OrphanOne {
				id: ID!
				two: OrphanTwo
			}

			type OrphanTwo {
				id: ID!
			}
		`,
			[]plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"me"}},
			},
			[]plan.TypeField{
				{TypeName: "User", FieldNames: []string{"id"}},
			})
	})
	t.Run("orphan cycle", func(t *testing.T) {
		run(t, `
			extend type Query {
				me: User
			}

			type User {
				id: ID!
			}

			# This type isn't connected to a root node, so
			# it doesn't show up as a child node.
			type OrphanOne {
				id: ID!
				two: OrphanTwo
			}

			type OrphanTwo {
				id: ID!
				one: OrphanOne
			}
		`,
			[]plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"me"}},
			},
			[]plan.TypeField{
				{TypeName: "User", FieldNames: []string{"id"}},
			})
	})
	t.Run("nested child nodes", func(t *testing.T) {
		run(t, `
			extend type Query {
				me: User
				review(id: ID!): Review
				user(id: ID!): User
			}

			type User {
				id: ID!
				reviews: [Review!]!
			}

			type Review {
				id: ID!
				comment: String!
				rating: Int!
				user: User
			}
		`,
			[]plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"me", "review", "user"}},
			},
			[]plan.TypeField{
				{TypeName: "Review", FieldNames: []string{"comment", "id", "rating", "user"}},
				{TypeName: "User", FieldNames: []string{"id", "reviews"}},
			})
	})
	t.Run("child node only available via nested child", func(t *testing.T) {
		run(t, `
			extend type Query {
				me: User
			}

			type User {
				id: ID!
				reviews: [Review!]!
			}

			# The Review type is connected to the Query root node via the User
			# type. It should therefore be found and included as a child node.
			type Review {
				id: ID!
				comment: String!
				rating: Int!
				user: User
			}
		`,
			[]plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"me"}},
			},
			[]plan.TypeField{
				{TypeName: "Review", FieldNames: []string{"comment", "id", "rating", "user"}},
				{TypeName: "User", FieldNames: []string{"id", "reviews"}},
			})
	})
	t.Run("interface", func(t *testing.T) {
		run(t, `
			extend type Query {
				me: User
				communication(id: ID!): Communication
				user(id: ID!): User
			}

			type User {
				id: ID!
				communications: [Communication!]!
			}

			type Review implements Communication {
				id: ID!
				comment: String!
				rating: Int!
				user: User
			}

			type Comment implements Communication {
				id: ID!
				comment: String!
				user: User
			}

			interface Communication {
				id: ID!
				comment: String!
				user: User
			}
		`,
			[]plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"communication", "me", "user"}},
			},
			[]plan.TypeField{
				{TypeName: "Comment", FieldNames: []string{"comment", "id", "user"}},
				{TypeName: "Communication", FieldNames: []string{"comment", "id", "user"}},
				{TypeName: "Review", FieldNames: []string{"comment", "id", "rating", "user"}},
				{TypeName: "User", FieldNames: []string{"communications", "id"}},
			})
	})
	t.Run("interface with key directive", func(t *testing.T) {
		run(t, `
			extend type Query {
				me: User
				communication(id: ID!): Communication
				user(id: ID!): User
			}

			type User {
				id: ID!
				communications: [Communication!]!
			}

			type Review implements Communication @key(fields: "id") {
				id: ID!
				comment: String!
				rating: Int!
				user: User
			}

			type Comment implements Communication @key(fields: "id") {
				id: ID!
				comment: String!
				user: User
			}

			# A key directive on an interface is allowed, but it doesn't make
			# the interface a root node. Entity queries can only be made for
			# concrete types.
			interface Communication @key(fields: "id") {
				id: ID!
				comment: String!
				user: User
			}
		`,
			[]plan.TypeField{
				{TypeName: "Comment", FieldNames: []string{"comment", "id", "user"}},
				{TypeName: "Query", FieldNames: []string{"communication", "me", "user"}},
				{TypeName: "Review", FieldNames: []string{"comment", "id", "rating", "user"}},
			},
			[]plan.TypeField{
				{TypeName: "Comment", FieldNames: []string{"comment", "id", "user"}},
				{TypeName: "Communication", FieldNames: []string{"comment", "id", "user"}},
				{TypeName: "Review", FieldNames: []string{"comment", "id", "rating", "user"}},
				{TypeName: "User", FieldNames: []string{"communications", "id"}},
			})
	})
	t.Run("extended interface", func(t *testing.T) {
		t.Log("Bug: The concrete types that implement an interface should also be included")

		run(t, `
			extend type Query {
				me: User
				communication(id: ID!): Communication
				user(id: ID!): User
			}

			type User {
				id: ID!
				communications: [Communication!]!
			}

			extend type Review implements Communication @key(fields: "id") {
				id: ID! @external
				comment: String!
				rating: Int!
				user: User
			}

			extend type Comment implements Communication @key(fields: "id") {
				id: ID! @external
				comment: String!
				user: User
			}

			extend interface Communication @key(fields: "id") {
				id: ID! @external
				comment: String!
				user: User
			}
		`,
			[]plan.TypeField{
				{TypeName: "Comment", FieldNames: []string{"comment", "user"}},
				{TypeName: "Query", FieldNames: []string{"communication", "me", "user"}},
				{TypeName: "Review", FieldNames: []string{"comment", "rating", "user"}},
			},
			[]plan.TypeField{
				{TypeName: "Comment", FieldNames: []string{"comment", "id", "user"}},
				{TypeName: "Communication", FieldNames: []string{"comment", "id", "user"}},
				{TypeName: "Review", FieldNames: []string{"comment", "id", "rating", "user"}},
				{TypeName: "User", FieldNames: []string{"communications", "id"}},
			})
	})
	t.Run("union", func(t *testing.T) {
		run(t, `
			extend type Query {
				me: User
				communication(id: ID!): Communication
				user(id: ID!): User
			}

			type User {
				id: ID!
				communications: [Communication!]!
			}

			type Review {
				id: ID!
				comment: String!
				rating: Int!
				user: User
			}

			type Comment {
				id: ID!
				comment: String!
				user: User
			}

			union Communication = Review | Comment
		`,
			[]plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"communication", "me", "user"}},
			},
			[]plan.TypeField{
				{TypeName: "Comment", FieldNames: []string{"comment", "id", "user"}},
				{TypeName: "Review", FieldNames: []string{"comment", "id", "rating", "user"}},
				{TypeName: "User", FieldNames: []string{"communications", "id"}},
			})
	})
	t.Run("union + interface", func(t *testing.T) {
		run(t, `
			type Query {
				histories: [History]
			}
			
			union History = Purchase | Sale
			
			interface Info {
				quantity: Int!
			}
			
			type Purchase implements Info {
				quantity: Int!
			}
			
			interface Store {
				location: String!
			}
			
			type Sale implements Store {
				location: String!
			}
		`,
			[]plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"histories"}},
			},
			[]plan.TypeField{
				{TypeName: "Info", FieldNames: []string{"quantity"}},
				{TypeName: "Purchase", FieldNames: []string{"quantity"}},
				{TypeName: "Sale", FieldNames: []string{"location"}},
				{TypeName: "Store", FieldNames: []string{"location"}},
			})
	})
	t.Run("extended union", func(t *testing.T) {
		run(t, `
			extend type Query {
				me: User
				communication(id: ID!): Communication
				user(id: ID!): User
			}

			type User {
				id: ID!
				communications: [Communication!]!
			}

			extend type Review @key(fields: "id") {
				id: ID! @external
				comment: String!
				rating: Int!
				user: User
			}

			extend type Comment @key(fields: "id") {
				id: ID! @external
				comment: String!
				user: User
			}

			extend union Communication = Review | Comment
		`,
			[]plan.TypeField{
				{TypeName: "Comment", FieldNames: []string{"comment", "user"}},
				{TypeName: "Query", FieldNames: []string{"communication", "me", "user"}},
				{TypeName: "Review", FieldNames: []string{"comment", "rating", "user"}},
			},
			[]plan.TypeField{
				{TypeName: "Comment", FieldNames: []string{"comment", "id", "user"}},
				{TypeName: "Review", FieldNames: []string{"comment", "id", "rating", "user"}},
				{TypeName: "User", FieldNames: []string{"communications", "id"}},
			})
	})
	t.Run("local union extension", func(t *testing.T) {
		run(t, `
			extend type Query {
				me: User
				communication(id: ID!): Communication
				user(id: ID!): User
			}

			type User {
				id: ID!
				communications: [Communication!]!
			}

			type Review {
				id: ID! @external
				comment: String!
				rating: Int!
				user: User
			}

			type Comment {
				id: ID! @external
				comment: String!
				user: User
			}

			type Post {
				id: ID!
				content: String!
			}

			union Communication = Review | Comment

			extend union Communication = Post
		`,
			[]plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"communication", "me", "user"}},
			},
			[]plan.TypeField{
				{TypeName: "Comment", FieldNames: []string{"comment", "id", "user"}},
				{TypeName: "Post", FieldNames: []string{"content", "id"}},
				{TypeName: "Review", FieldNames: []string{"comment", "id", "rating", "user"}},
				{TypeName: "User", FieldNames: []string{"communications", "id"}},
			})
	})
	t.Run("Entity definition", func(t *testing.T) {
		run(t, `
			type User @key(fields: "id") {
				id: ID!
				reviews: [Review!]!
			}

			type Review {
				id: ID!
				comment: String!
				rating: Int!
			}
		`,
			[]plan.TypeField{
				{TypeName: "User", FieldNames: []string{"id", "reviews"}},
			},
			[]plan.TypeField{
				{TypeName: "Review", FieldNames: []string{"comment", "id", "rating"}},
			})
	})
	t.Run("nested Entity definition", func(t *testing.T) {
		run(t, `
			extend type Query {
				me: User
			}

			type User @key(fields: "id") {
				id: ID!
				reviews: [Review!]!
			}

			type Review {
				id: ID!
				comment: String!
				rating: Int!
			}
		`,
			[]plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"me"}},
				{TypeName: "User", FieldNames: []string{"id", "reviews"}},
			},
			[]plan.TypeField{
				{TypeName: "Review", FieldNames: []string{"comment", "id", "rating"}},
				{TypeName: "User", FieldNames: []string{"id", "reviews"}},
			})
	})
	t.Run("extended Entity", func(t *testing.T) {
		run(t, `
			extend type User @key(fields: "id") {
				id: ID! @external
				username: String! @external
				reviews: [Review!]
			}

			type Review {
				comment: String!
				author: User! @provide(fields: "username")
			}
		`,
			[]plan.TypeField{
				{TypeName: "User", FieldNames: []string{"reviews"}},
			},
			[]plan.TypeField{
				{TypeName: "Review", FieldNames: []string{"author", "comment"}},
				{TypeName: "User", FieldNames: []string{"id", "reviews", "username"}},
			})
	})
	t.Run("extended Entity without local fields", func(t *testing.T) {
		run(t, `
			extend type Query {
				review(id: ID!): Review
			}

			# This entity doesn't define any local fields, so it shouldn't be
			# included as a root node.
			extend type User @key(fields: "id") {
				id: ID! @external
			}

			type Review {
				comment: String!
				author: User!
			}
		`,
			[]plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"review"}},
			},
			[]plan.TypeField{
				{TypeName: "Review", FieldNames: []string{"author", "comment"}},
				{TypeName: "User", FieldNames: []string{"id"}},
			})
	})
	t.Run("extended Entity with root definitions", func(t *testing.T) {
		run(t, `
			extend type Query {
				reviews(IDs: [ID!]!): [Review!]
			}

			extend type User @key(fields: "id") {
				id: ID! @external
				reviews: [Review!]
			}

			type Review {
				id: String!
				comment: String!
				author: User!
			}
		`,
			[]plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"reviews"}},
				{TypeName: "User", FieldNames: []string{"reviews"}},
			},
			[]plan.TypeField{
				{TypeName: "Review", FieldNames: []string{"author", "comment", "id"}},
				{TypeName: "User", FieldNames: []string{"id", "reviews"}},
			})
	})
	t.Run("extended Entity with required fields", func(t *testing.T) {
		run(t, `
			extend type User @key(fields: "id") {
				id: ID! @external
				username: String! @external
				lastname: String! @external

				reviews: [Review!]
				fullname: String @requires(fields: "{ lastname }")
			}

			type Review {
				comment: String!
				author: User! @provide(fields: "username")
			}
		`,
			[]plan.TypeField{
				{TypeName: "User", FieldNames: []string{"fullname", "reviews"}},
			},
			[]plan.TypeField{
				{TypeName: "Review", FieldNames: []string{"author", "comment"}},
				{TypeName: "User", FieldNames: []string{"fullname", "id", "reviews", "username"}},
			})
	})
	t.Run("local type extension", func(t *testing.T) {
		run(t, `
           extend type Query {
                   reviews(IDs: [ID!]!): [Review!]
                   products(IDs: [ID!]!): [Product!]
           }

           extend type User @key(fields: "id") {
                   id: ID! @external
                   reviews: [Review!]
           }

           type Review {
                   id: String!
                   author: User!
           }

           extend type Review {
                   comment: String!
           }

           # Product is an owned federated type that also has a local type
           # extension.

           type Product @key(fields: "id") {
                   id: ID!
                   addedBy: User!
           }

           extend type Product {
                   description: String!
           }
       `,
			[]plan.TypeField{
				{TypeName: "Product", FieldNames: []string{"addedBy", "description", "id"}},
				{TypeName: "Query", FieldNames: []string{"products", "reviews"}},
				{TypeName: "User", FieldNames: []string{"reviews"}},
			},
			[]plan.TypeField{
				{TypeName: "Product", FieldNames: []string{"addedBy", "description", "id"}},
				{TypeName: "Review", FieldNames: []string{"author", "comment", "id"}},
				{TypeName: "User", FieldNames: []string{"id", "reviews"}},
			})
	})
	t.Run("local type extension defined before local type", func(t *testing.T) {
		run(t, `
           extend type Query {
                   reviews(IDs: [ID!]!): [Review!]
                   products(IDs: [ID!]!): [Product!]
           }

           extend type User @key(fields: "id") {
                   id: ID! @external
                   reviews: [Review!]
           }

           extend type Review {
                   comment: String!
           }

           type Review {
                   id: ID!
                   author: User!
           }

           # Product is an owned federated type that also has a local type
           # extension.

           extend type Product {
                   description: String!
           }

           type Product @key(fields: "id") {
                   id: ID!
                   addedBy: User!
           }
       `,
			[]plan.TypeField{
				{TypeName: "Product", FieldNames: []string{"addedBy", "description", "id"}},
				{TypeName: "Query", FieldNames: []string{"products", "reviews"}},
				{TypeName: "User", FieldNames: []string{"reviews"}},
			},
			[]plan.TypeField{
				{TypeName: "Product", FieldNames: []string{"addedBy", "description", "id"}},
				{TypeName: "Review", FieldNames: []string{"author", "comment", "id"}},
				{TypeName: "User", FieldNames: []string{"id", "reviews"}},
			})
	})
	t.Run("union types", func(t *testing.T) {
		run(t, `
			extend type Query {
				search(name: String!): SearchResult
			}

			union SearchResult = Human | Droid | Starship
	
			interface Character {
				name: String!
				friends: [Character]
			}
			
			type Human implements Character {
				name: String!
				height: String!
				friends: [Character]
			}
			
			type Droid implements Character {
				name: String!
				primaryFunction: String!
				friends: [Character]
			}
			
			type Starship {
				name: String!
				length: Float!
			}
		`,
			[]plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"search"}},
			},
			[]plan.TypeField{
				{TypeName: "Character", FieldNames: []string{"friends", "name"}},
				{TypeName: "Droid", FieldNames: []string{"friends", "name", "primaryFunction"}},
				{TypeName: "Human", FieldNames: []string{"friends", "height", "name"}},
				{TypeName: "Starship", FieldNames: []string{"length", "name"}},
			})
	})
	t.Run("interface types", func(t *testing.T) {
		run(t, `
			extend type Query {
				search(name: String!): Character
			}
	
			interface Character {
				name: String!
				friends: [Character]
			}
			
			type Human implements Character {
				name: String!
				height: String!
				friends: [Character]
			}
			
			type Droid implements Character {
				name: String!
				primaryFunction: String!
				friends: [Character]
			}
		`,
			[]plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"search"}},
			},
			[]plan.TypeField{
				{TypeName: "Character", FieldNames: []string{"friends", "name"}},
				{TypeName: "Droid", FieldNames: []string{"friends", "name", "primaryFunction"}},
				{TypeName: "Human", FieldNames: []string{"friends", "height", "name"}},
			})
	})
}

func BenchmarkGetAllNodes(b *testing.B) {
	document := unsafeparser.ParseGraphqlDocumentString(benchmarkSDL)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		extractor := NewLocalTypeFieldExtractor(&document)
		extractor.GetAllNodes()
	}
}

const benchmarkSDL = `
	extend type Query {
		ownedA: OwnedA!
		ownedB: OwnedB!
		ownedC: OwnedC!
		ownedD: OwnedD!
		ownedE: OwnedE!
		ownedF: OwnedF!
		ownedG: OwnedG!
		ownedH: OwnedH!
		ownedI: OwnedI!
		ownedJ: OwnedJ!
		ownedK: OwnedK!
		ownedL: OwnedL!
		ownedM: OwnedM!
		ownedN: OwnedN!
		ownedO: OwnedO!
		ownedP: OwnedP!
		extendedA: ExtendedA!
		extendedB: ExtendedB!
		extendedC: ExtendedC!
		extendedD: ExtendedD!
		extendedE: ExtendedE!
		extendedF: ExtendedF!
		extendedG: ExtendedG!
		extendedH: ExtendedH!
	}

	type OwnedA {
		id: ID!
		fieldOne: String!
		fieldTwo: String!
		nextOwnedType: OwnedB!
	}

	type OwnedB {
		id: ID!
		fieldOne: String!
		fieldTwo: String!
		nextOwnedType: OwnedC!
	}

	type OwnedC {
		id: ID!
		fieldOne: String!
		fieldTwo: String!
		nextOwnedType: OwnedD!
	}

	type OwnedD {
		id: ID!
		fieldOne: String!
		fieldTwo: String!
		nextOwnedType: OwnedE!
	}

	type OwnedE {
		id: ID!
		fieldOne: String!
		fieldTwo: String!
		nextOwnedType: OwnedF!
	}

	type OwnedF {
		id: ID!
		fieldOne: String!
		fieldTwo: String!
		nextOwnedType: OwnedG!
	}

	type OwnedG {
		id: ID!
		fieldOne: String!
		fieldTwo: String!
		nextOwnedType: OwnedH!
	}

	type OwnedH {
		id: ID!
		fieldOne: String!
		fieldTwo: String!
		nextOwnedType: OwnedI!
	}

	type OwnedI {
		id: ID!
		fieldOne: String!
		fieldTwo: String!
		nextOwnedType: OwnedJ!
	}

	type OwnedJ {
		id: ID!
		fieldOne: String!
		fieldTwo: String!
		nextOwnedType: OwnedK!
	}

	type OwnedK {
		id: ID!
		fieldOne: String!
		fieldTwo: String!
		nextOwnedType: OwnedL!
	}

	type OwnedL {
		id: ID!
		fieldOne: String!
		fieldTwo: String!
		nextOwnedType: OwnedM!
	}

	type OwnedM {
		id: ID!
		fieldOne: String!
		fieldTwo: String!
		nextOwnedType: OwnedN!
	}

	type OwnedN {
		id: ID!
		fieldOne: String!
		fieldTwo: String!
		nextOwnedType: OwnedO!
	}

	type OwnedO {
		id: ID!
		fieldOne: String!
		fieldTwo: String!
		nextOwnedType: OwnedP!
	}

	type OwnedP {
		id: ID!
		fieldOne: String!
		fieldTwo: String!
		nextOwnedType: OwnedA!
	}

	extend type ExtendedA {
		id: ID! @external
		fieldOne: String!
		fieldTwo: String!
		nextExtendedType: ExtendedB!
	}

	extend type ExtendedB {
		id: ID! @external
		fieldOne: String!
		fieldTwo: String!
		nextExtendedType: ExtendedC!
	}

	extend type ExtendedC {
		id: ID! @external
		fieldOne: String!
		fieldTwo: String!
		nextExtendedType: ExtendedD!
	}

	extend type ExtendedD {
		id: ID! @external
		fieldOne: String!
		fieldTwo: String!
		nextExtendedType: ExtendedE!
	}

	extend type ExtendedE {
		id: ID! @external
		fieldOne: String!
		fieldTwo: String!
		nextExtendedType: ExtendedF!
	}

	extend type ExtendedF {
		id: ID! @external
		fieldOne: String!
		fieldTwo: String!
		nextExtendedType: ExtendedG!
	}

	extend type ExtendedG {
		id: ID! @external
		fieldOne: String!
		fieldTwo: String!
		nextExtendedType: ExtendedH!
	}

	extend type ExtendedH {
		id: ID! @external
		fieldOne: String!
		fieldTwo: String!
		nextExtendedType: ExtendedA!
	}
`
