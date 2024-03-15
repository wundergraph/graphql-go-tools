package plan

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/TykTechnologies/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/operationreport"
)

type dsBuilder struct {
	ds *DataSourceConfiguration
}

func dsb() *dsBuilder { return &dsBuilder{ds: &DataSourceConfiguration{}} }

func (b *dsBuilder) RootNode(typeName string, fieldNames ...string) *dsBuilder {
	b.ds.RootNodes = append(b.ds.RootNodes, TypeField{TypeName: typeName, FieldNames: fieldNames})
	return b
}

func (b *dsBuilder) ChildNode(typeName string, fieldNames ...string) *dsBuilder {
	b.ds.ChildNodes = append(b.ds.ChildNodes, TypeField{TypeName: typeName, FieldNames: fieldNames})
	return b
}

func (b *dsBuilder) Schema(schema string) *dsBuilder {
	b.ds.Custom = []byte(schema)
	return b
}

func (b *dsBuilder) KeysMetadata(keys FederationFieldConfigurations) *dsBuilder {
	b.ds.FederationMetaData.Keys = keys
	return b
}

func (b *dsBuilder) Hash(hash DSHash) *dsBuilder {
	b.ds.hash = hash
	return b
}

func (b *dsBuilder) DS() DataSourceConfiguration {
	if len(b.ds.Custom) == 0 {
		panic("schema not set")
	}
	b.ds.Hash()
	return *b.ds
}

func (b *dsBuilder) DSPtr() *DataSourceConfiguration {
	return b.ds
}

func strptr(s string) *string { return &s }

func TestNodeSuggestions(t *testing.T) {
	t.Run("isNodeUniq", func(t *testing.T) {
		nodes := NodeSuggestions{
			{TypeName: "Query", FieldName: "user", DataSourceHash: 11, Path: "query.user", ParentPath: "query"},
			{TypeName: "User", FieldName: "id", DataSourceHash: 11, Path: "query.user.id", ParentPath: "query.user"},
			{TypeName: "User", FieldName: "id", DataSourceHash: 22, Path: "query.user.id", ParentPath: "query.user"},
		}

		assert.False(t, nodes.isNodeUniq(1))
		assert.True(t, nodes.isNodeUniq(0))
	})

	t.Run("isSelectedOnOtherSource", func(t *testing.T) {
		nodes := NodeSuggestions{
			{TypeName: "Query", FieldName: "user", DataSourceHash: 11, Path: "query.user", ParentPath: "query"},
			{TypeName: "Query", FieldName: "user", DataSourceHash: 22, Path: "query.user", ParentPath: "query", selected: true},
			{TypeName: "User", FieldName: "id", DataSourceHash: 11, Path: "query.user.id", ParentPath: "query.user"},
			{TypeName: "User", FieldName: "id", DataSourceHash: 22, Path: "query.user.id", ParentPath: "query.user"},
		}

		assert.True(t, nodes.isSelectedOnOtherSource(0))
		assert.False(t, nodes.isSelectedOnOtherSource(2))
	})

	t.Run("duplicatesOf", func(t *testing.T) {
		nodes := NodeSuggestions{
			{TypeName: "Query", FieldName: "user", DataSourceHash: 11, Path: "query.user", ParentPath: "query"},
			{TypeName: "User", FieldName: "id", DataSourceHash: 11, Path: "query.user.id", ParentPath: "query.user"},
			{TypeName: "User", FieldName: "id", DataSourceHash: 22, Path: "query.user.id", ParentPath: "query.user"},
			{TypeName: "User", FieldName: "id", DataSourceHash: 33, Path: "query.user.id", ParentPath: "query.user"},
		}

		assert.Equal(t, []int{2, 3}, nodes.duplicatesOf(1))
		assert.Nil(t, nodes.duplicatesOf(0))
	})

	t.Run("childNodesOnSameSource", func(t *testing.T) {
		nodes := NodeSuggestions{
			{TypeName: "Query", FieldName: "user", DataSourceHash: 11, Path: "query.user", ParentPath: "query"},
			{TypeName: "User", FieldName: "id", DataSourceHash: 11, Path: "query.user.$User.id", ParentPath: "query.user", parentPathWithoutFragment: strptr("query.user"), onFragment: true},
			{TypeName: "User", FieldName: "info", DataSourceHash: 11, Path: "query.user.$User.info", ParentPath: "query.user.$User", parentPathWithoutFragment: strptr("query.user"), onFragment: true},
			{TypeName: "Info", FieldName: "email", DataSourceHash: 11, Path: "query.user.$User.info.email", ParentPath: "query.user.$User.info", parentPathWithoutFragment: strptr("query.user.$User.info")},
			{TypeName: "Info", FieldName: "isUser", DataSourceHash: 33, Path: "query.user.$User.info.isUser", ParentPath: "query.user.$User.info", parentPathWithoutFragment: strptr("query.user.$User.info")},
			{TypeName: "User", FieldName: "isAdmin", DataSourceHash: 44, Path: "query.user.isAdmin", ParentPath: "query.user", parentPathWithoutFragment: strptr("query.user"), onFragment: true},
		}

		assert.Equal(t, []int{1, 2}, nodes.childNodesOnSameSource(0))
		assert.Nil(t, nodes.childNodesOnSameSource(1))
		assert.Equal(t, []int{3}, nodes.childNodesOnSameSource(2))
	})

	t.Run("siblingNodesOnSameSource", func(t *testing.T) {
		nodes := NodeSuggestions{
			{TypeName: "Query", FieldName: "user", DataSourceHash: 11, Path: "query.user", ParentPath: "query"},
			{TypeName: "User", FieldName: "id", DataSourceHash: 11, Path: "query.user.$User.id", ParentPath: "query.user", parentPathWithoutFragment: strptr("query.user"), onFragment: true},
			{TypeName: "User", FieldName: "info", DataSourceHash: 11, Path: "query.user.$User.info", ParentPath: "query.user.$User", parentPathWithoutFragment: strptr("query.user"), onFragment: true},
			{TypeName: "Info", FieldName: "email", DataSourceHash: 11, Path: "query.user.$User.info.email", ParentPath: "query.user.$User.info"},
			{TypeName: "Info", FieldName: "isUser", DataSourceHash: 33, Path: "query.user.$User.info.isUser", ParentPath: "query.user.$User.info"},
			{TypeName: "User", FieldName: "isAdmin", DataSourceHash: 44, Path: "query.user.isAdmin", ParentPath: "query.user"},
			{TypeName: "Moderator", FieldName: "isModerator", DataSourceHash: 11, Path: "query.user.$Moderator.isModerator", ParentPath: "query.user.$Moderator", parentPathWithoutFragment: strptr("query.user"), onFragment: true},
		}

		assert.Nil(t, nodes.siblingNodesOnSameSource(0))
		assert.Nil(t, nodes.siblingNodesOnSameSource(4))
		assert.Nil(t, nodes.siblingNodesOnSameSource(5))
		assert.Equal(t, []int{2, 6}, nodes.siblingNodesOnSameSource(1))
	})

	t.Run("isLeaf", func(t *testing.T) {
		nodes := NodeSuggestions{
			{TypeName: "Query", FieldName: "user", DataSourceHash: 11, Path: "query.user", ParentPath: "query"},
			{TypeName: "User", FieldName: "id", DataSourceHash: 11, Path: "query.user.id", ParentPath: "query.user"},
			{TypeName: "User", FieldName: "id", DataSourceHash: 22, Path: "query.user.id", ParentPath: "query.user"},
			{TypeName: "User", FieldName: "id", DataSourceHash: 33, Path: "query.user.id", ParentPath: "query.user"},
			{TypeName: "User", FieldName: "info", DataSourceHash: 11, Path: "query.user.info", ParentPath: "query.user"},
			{TypeName: "Info", FieldName: "email", DataSourceHash: 11, Path: "query.user.info.email", ParentPath: "query.user.info"},
		}

		assert.False(t, nodes.isLeaf(0))
		assert.True(t, nodes.isLeaf(1))
		assert.True(t, nodes.isLeaf(2))
		assert.True(t, nodes.isLeaf(3))
		assert.False(t, nodes.isLeaf(4))
		assert.True(t, nodes.isLeaf(5))
	})

	t.Run("parentNodeOnSameSource", func(t *testing.T) {
		t.Run("no fragments", func(t *testing.T) {
			nodes := NodeSuggestions{
				{TypeName: "Query", FieldName: "user", DataSourceHash: 11, Path: "query.user", ParentPath: "query"},
				{TypeName: "User", FieldName: "id", DataSourceHash: 11, Path: "query.user.id", ParentPath: "query.user"},
				{TypeName: "User", FieldName: "info", DataSourceHash: 22, Path: "query.user.info", ParentPath: "query.user"},
				{TypeName: "Info", FieldName: "email", DataSourceHash: 22, Path: "query.user.info.email", ParentPath: "query.user.info"},
			}

			cases := []struct {
				parentIdx int
				ok        bool
			}{
				{-1, false},
				{0, true},
				{-1, false},
				{2, true},
			}

			for i, c := range cases {
				parentIdx, ok := nodes.parentNodeOnSameSource(i)
				assert.Equal(t, c.parentIdx, parentIdx, "case %d", i)
				assert.Equal(t, c.ok, ok, "case %d", i)
			}
		})

		t.Run("with fragments", func(t *testing.T) {
			nodes := NodeSuggestions{
				{TypeName: "Query", FieldName: "user", DataSourceHash: 11, Path: "query.user", ParentPath: "query"},
				{TypeName: "User", FieldName: "id", DataSourceHash: 11, Path: "query.user.$User.id", ParentPath: "query.user", parentPathWithoutFragment: strptr("query.user"), onFragment: true},
				{TypeName: "User", FieldName: "info", DataSourceHash: 11, Path: "query.user.$User.info", ParentPath: "query.user.$User", parentPathWithoutFragment: strptr("query.user"), onFragment: true},
				{TypeName: "Info", FieldName: "email", DataSourceHash: 11, Path: "query.user.$User.info.email", ParentPath: "query.user.$User.info"},
				{TypeName: "Info", FieldName: "isUser", DataSourceHash: 33, Path: "query.user.$User.info.isUser", ParentPath: "query.user.$User.info"},
				{TypeName: "Admin", FieldName: "id", DataSourceHash: 22, Path: "query.user.$Admin.id", ParentPath: "query.user.$Admin", parentPathWithoutFragment: strptr("query.user"), onFragment: true},
				{TypeName: "Admin", FieldName: "isAdmin", DataSourceHash: 44, Path: "query.user.$Admin.id", ParentPath: "query.user.$Admin", parentPathWithoutFragment: strptr("query.user"), onFragment: true},
			}

			cases := []struct {
				parentIdx int
				ok        bool
			}{
				{-1, false},
				{0, true},
				{0, true},
				{2, true},
				{-1, false},
				{-1, false},
				{-1, false},
			}

			for i, c := range cases {
				parentIdx, ok := nodes.parentNodeOnSameSource(i)
				assert.Equal(t, c.parentIdx, parentIdx, "case %d", i)
				assert.Equal(t, c.ok, ok, "case %d", i)
			}
		})
	})

}

func TestFindBestDataSourceSet(t *testing.T) {
	type Variant struct {
		dsOrder     []int
		suggestions NodeSuggestions
	}

	type TestCase struct {
		Description         string
		Definition          string
		Query               string
		DataSources         []DataSourceConfiguration
		ExpectedVariants    []Variant
		ExpectedSuggestions NodeSuggestions
	}

	testCases := []TestCase{
		{
			Description: "Choose users from first data source and name from the second",
			Definition: `
				type Query {
					provider: AccountProvider
				}
				union Account = User
				type AccountProvider {
					accounts: [Account]
		        }
				type User {
					id: Int
					name: String
				}	
			`,
			Query: `
				query {
					provider {
						accounts {
							... on User {
								name
							}
						}
					}
				}
			`,
			DataSources: []DataSourceConfiguration{
				dsb().Hash(22).Schema(`
					union Account = User
					type AccountProvider {
						accounts: [Account]
					}
					type User @key(fields: "id") {
						id: Int
						name: String
					}
				`).RootNode("User", "id", "name").
					ChildNode("AccountProvider", "accounts").DS(),
				dsb().Hash(11).Schema(`
					type Query {
						provider: AccountProvider
					}
					union Account = User
					type AccountProvider {
						accounts: [Account]
					}
					type User @key(fields: "id") {
						id: Int
					}
				`).RootNode("Query", "provider").
					ChildNode("AccountProvider", "accounts").
					RootNode("User", "id").DS(),
			},
			ExpectedSuggestions: NodeSuggestions{
				{TypeName: "Query", FieldName: "provider", DataSourceHash: 11, Path: "query.provider", ParentPath: "query", IsRootNode: true, selected: true},
				{TypeName: "AccountProvider", FieldName: "accounts", DataSourceHash: 11, Path: "query.provider.accounts", ParentPath: "query.provider", selected: true},
				{TypeName: "User", FieldName: "name", DataSourceHash: 22, Path: "query.provider.accounts.$User.name", ParentPath: "query.provider.accounts.$User", onFragment: true, parentPathWithoutFragment: strptr("query.provider.accounts"), IsRootNode: true, selected: true},
			},
		},
		{
			Description: "Remove the 3rd data source, we don't use it",
			Definition: `
				type Query {
					user: User
				}
				type User {
					id: Int
					name: String
					surname: String
					age: Int
				}	
			`,
			Query: `
				query {
					user {
						id
						name
						surname
					}
				}
			`,
			DataSources: []DataSourceConfiguration{
				dsb().Hash(11).Schema(`
					type Query {
						user: User
					}
					type User @key(fields: "id") {
						id: Int
					}
				`).RootNode("Query", "user").RootNode("User", "id").DS(),
				dsb().Hash(22).Schema(`
					type User @key(fields: "id") {
						id: Int
						name: String
						surname: String
					}
				`).RootNode("User", "id", "name", "surname").DS(),
				dsb().Hash(33).Schema(`
					type User @key(fields: "id") {
						id: Int
						age: Int
					}
				`).RootNode("User", "id", "age").DS(),
			},
			ExpectedSuggestions: NodeSuggestions{
				{TypeName: "Query", FieldName: "user", DataSourceHash: 11, Path: "query.user", ParentPath: "query", IsRootNode: true, selected: true},
				{TypeName: "User", FieldName: "id", DataSourceHash: 11, Path: "query.user.id", ParentPath: "query.user", IsRootNode: true, selected: true},
				{TypeName: "User", FieldName: "name", DataSourceHash: 22, Path: "query.user.name", ParentPath: "query.user", IsRootNode: true, selected: true},
				{TypeName: "User", FieldName: "surname", DataSourceHash: 22, Path: "query.user.surname", ParentPath: "query.user", IsRootNode: true, selected: true},
			},
		},
		{
			Description: "Pick the first and the third data sources, ignore the ones that result in more queries",
			Definition: `
				type Query {
					user: User
				}
				type User {
					id: Int
					age: Int
					name: String
					surname: String
				}	
			`,
			Query: `
				query {
					user {
						age
						name
						surname
					}
				}
			`,
			DataSources: []DataSourceConfiguration{
				dsb().Hash(11).Schema(`
					type Query {
						user: User
					}
					type User @key(fields: "id") {
						id: Int
						age: Int
					}
				`).RootNode("Query", "user").RootNode("User", "id", "age").DS(),
				dsb().Hash(22).Schema(`
					type User @key(fields: "id") {
						id: Int
						age: Int
						name: String
					}
				`).RootNode("User", "id", "age", "name").DS(),
				dsb().Hash(33).Schema(`
					type User @key(fields: "id") {
						id: Int
						name: String
						surname: String
					}
				`).RootNode("User", "id", "name", "surname").DS(),
				dsb().Hash(44).Schema(`
					type User @key(fields: "id") {
						id: Int
						age: Int
					}
				`).RootNode("User", "id", "age").DS(),
				dsb().Hash(55).Schema(`
					type User @key(fields: "id") {
						id: Int
						name: String
					}
				`).RootNode("User", "id", "name").DS(),
			},
			ExpectedSuggestions: NodeSuggestions{
				{TypeName: "Query", FieldName: "user", DataSourceHash: 11, Path: "query.user", ParentPath: "query", IsRootNode: true, selected: true},
				{TypeName: "User", FieldName: "age", DataSourceHash: 11, Path: "query.user.age", ParentPath: "query.user", IsRootNode: true, selected: true},
				{TypeName: "User", FieldName: "name", DataSourceHash: 33, Path: "query.user.name", ParentPath: "query.user", IsRootNode: true, selected: true},
				{TypeName: "User", FieldName: "surname", DataSourceHash: 33, Path: "query.user.surname", ParentPath: "query.user", IsRootNode: true, selected: true},
			},
		},
		{
			Description: "Complex example. Entities: User, Address, Lines",
			Definition: `
						type Query {
							user: User
						}
						type User {
							id: String
							name: String
							details: Details
						}
						type Details {
							age: Int
							address: Address
						}
						type Address {
							id: String
							name: String
							lines: Lines
						}
						type Lines {
							id: String
							line1: String
							line2: String
						}
					`,
			Query: `
						query {
							user {
								name
								details {
									age
									address {
										name
										lines {
											# TODO: added key id to make test results predictable
											# revisit if it is not enough rules to help planner
											id 
											line1
											line2
										}
									}
								}
							}
						}
					`,
			DataSources: []DataSourceConfiguration{
				dsb().Hash(11).Schema(`
					# sub1
					type Query {
						user: User
					}
					type User @key(fields: "id") {
						id: Int
						name: String
					}
				`).
					RootNode("Query", "user").
					RootNode("User", "id", "name").DS(),
				dsb().Hash(22).Schema(`
					# sub2
					type Query {
						user: User
					}
					type User @key(fields: "id") {
						id: Int
						details: Details
						name: String
					}
					type Details {
						age: Int
						address: Address
					}
					type Address @key(fields: "id") {
						id: Int
						name: String
						lines: Lines
					}
					type Lines @key(fields: "id") {
						id: Int
					}
				`).
					RootNode("Query", "user").
					RootNode("User", "id", "details", "name").
					ChildNode("Details", "address", "age").
					RootNode("Address", "id", "name", "lines").
					RootNode("Lines", "id").
					DS(),
				dsb().Hash(33).Schema(`
					# sub3
					type Address @key(fields: "id") {
						id: Int
						lines: Lines
					}
					type Lines @key(fields: "id") {
						id: Int
						line1: String
					}
				`).
					RootNode("Address", "id", "lines").
					RootNode("Lines", "id", "line1").
					DS(),
				dsb().Hash(44).Schema(`
					# sub4
					type Lines @key(fields: "id") {
						id: Int
						line1: String
						line2: String
					}
				`).
					RootNode("Lines", "id", "line1", "line2").
					DS(),
			},
			ExpectedSuggestions: NodeSuggestions{
				{TypeName: "Query", FieldName: "user", DataSourceHash: 22, Path: "query.user", ParentPath: "query", IsRootNode: true, selected: true},
				{TypeName: "User", FieldName: "name", DataSourceHash: 22, Path: "query.user.name", ParentPath: "query.user", IsRootNode: true, selected: true},
				{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.user.details", ParentPath: "query.user", IsRootNode: true, selected: true},
				{TypeName: "Details", FieldName: "age", DataSourceHash: 22, Path: "query.user.details.age", ParentPath: "query.user.details", IsRootNode: false, selected: true},
				{TypeName: "Details", FieldName: "address", DataSourceHash: 22, Path: "query.user.details.address", ParentPath: "query.user.details", IsRootNode: false, selected: true},
				{TypeName: "Address", FieldName: "name", DataSourceHash: 22, Path: "query.user.details.address.name", ParentPath: "query.user.details.address", IsRootNode: true, selected: true},
				{TypeName: "Address", FieldName: "lines", DataSourceHash: 22, Path: "query.user.details.address.lines", ParentPath: "query.user.details.address", IsRootNode: true, selected: true},
				{TypeName: "Lines", FieldName: "id", DataSourceHash: 22, Path: "query.user.details.address.lines.id", ParentPath: "query.user.details.address.lines", IsRootNode: true, selected: true},
				{TypeName: "Lines", FieldName: "line1", DataSourceHash: 44, Path: "query.user.details.address.lines.line1", ParentPath: "query.user.details.address.lines", IsRootNode: true, selected: true},
				{TypeName: "Lines", FieldName: "line2", DataSourceHash: 44, Path: "query.user.details.address.lines.line2", ParentPath: "query.user.details.address.lines", IsRootNode: true, selected: true},
			},
		},
		{
			Description: "Shareable variant",
			Definition: `
				type Query {
					me: User
				}
				type User {
					id: Int
					details: Details
				}
				type Details {
					name: String
					surname: String
				}
			`,
			Query: `
				query {
					me {
						details {
							surname
							name
						}
					}
				}
			`,
			DataSources: []DataSourceConfiguration{
				dsb().Hash(11).Schema(`
					type Query {
						me: User
					}
					type User @key(fields: "id") {
						id: Int
						details: Details
					}
					type Details {
						name: String
					}
				`).
					RootNode("Query", "me").
					RootNode("User", "id", "details").
					ChildNode("Details", "name").
					DS(),
				dsb().Hash(22).Schema(`
					type User @key(fields: "id") {
						id: Int
						details: Details
					}
					type Details {
						surname: String
					}
				`).
					RootNode("User", "id", "details").
					ChildNode("Details", "surname").
					DS(),
			},
			ExpectedVariants: []Variant{
				{
					dsOrder: []int{0, 1},
					suggestions: NodeSuggestions{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 11, Path: "query.me", ParentPath: "query", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "details", DataSourceHash: 11, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
						{TypeName: "Details", FieldName: "surname", DataSourceHash: 22, Path: "query.me.details.surname", ParentPath: "query.me.details", IsRootNode: false, selected: true},
						{TypeName: "Details", FieldName: "name", DataSourceHash: 11, Path: "query.me.details.name", ParentPath: "query.me.details", IsRootNode: false, selected: true},
					},
				},
				{
					dsOrder: []int{1, 0},
					suggestions: NodeSuggestions{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 11, Path: "query.me", ParentPath: "query", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "details", DataSourceHash: 11, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
						{TypeName: "Details", FieldName: "surname", DataSourceHash: 22, Path: "query.me.details.surname", ParentPath: "query.me.details", IsRootNode: false, selected: true},
						{TypeName: "Details", FieldName: "name", DataSourceHash: 11, Path: "query.me.details.name", ParentPath: "query.me.details", IsRootNode: false, selected: true},
					},
				},
			},
		},
		{
			Description: "Shareable: 2 ds are equal so choose first available",
			Definition:  shareableDefinition,
			Query: `
				query {
					me {
						details {
							forename
						}
					}
				}
			`,
			DataSources: []DataSourceConfiguration{
				shareableDS1,
				shareableDS2,
				shareableDS3,
			},
			ExpectedVariants: []Variant{
				{
					dsOrder: []int{0, 1, 2},
					suggestions: NodeSuggestions{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 11, Path: "query.me", ParentPath: "query", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "details", DataSourceHash: 11, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
						{TypeName: "Details", FieldName: "forename", DataSourceHash: 11, Path: "query.me.details.forename", ParentPath: "query.me.details", IsRootNode: false, selected: true},
					},
				},
				{
					dsOrder: []int{2, 1, 0},
					suggestions: NodeSuggestions{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 22, Path: "query.me", ParentPath: "query", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
						{TypeName: "Details", FieldName: "forename", DataSourceHash: 22, Path: "query.me.details.forename", ParentPath: "query.me.details", IsRootNode: false, selected: true},
					},
				},
			},
		},
		{
			Description: "Shareable: choose second it provides more fields",
			Definition:  shareableDefinition,
			Query: `
				query {
					me {
						details {
							forename
							surname
						}
					}
				}
			`,
			DataSources: []DataSourceConfiguration{
				shareableDS1,
				shareableDS2,
				shareableDS3,
			},
			ExpectedSuggestions: NodeSuggestions{
				{TypeName: "Query", FieldName: "me", DataSourceHash: 22, Path: "query.me", ParentPath: "query", IsRootNode: true, selected: true},
				{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
				{TypeName: "Details", FieldName: "forename", DataSourceHash: 22, Path: "query.me.details.forename", ParentPath: "query.me.details", IsRootNode: false, selected: true},
				{TypeName: "Details", FieldName: "surname", DataSourceHash: 22, Path: "query.me.details.surname", ParentPath: "query.me.details", IsRootNode: false, selected: true},
			},
		},
		{
			Description: "Shareable: should use 2 ds",
			Definition:  shareableDefinition,
			Query: `
				query {
					me {
						details {
							forename
							surname
							middlename
						}
					}
				}
			`,
			DataSources: []DataSourceConfiguration{
				shareableDS1,
				shareableDS2,
				shareableDS3,
			},
			ExpectedVariants: []Variant{
				{
					dsOrder: []int{1, 0, 2},
					suggestions: NodeSuggestions{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 22, Path: "query.me", ParentPath: "query", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "details", DataSourceHash: 11, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
						{TypeName: "Details", FieldName: "forename", DataSourceHash: 22, Path: "query.me.details.forename", ParentPath: "query.me.details", IsRootNode: false, selected: true},
						{TypeName: "Details", FieldName: "surname", DataSourceHash: 22, Path: "query.me.details.surname", ParentPath: "query.me.details", IsRootNode: false, selected: true},
						{TypeName: "Details", FieldName: "middlename", DataSourceHash: 11, Path: "query.me.details.middlename", ParentPath: "query.me.details", IsRootNode: false, selected: true},
					},
				},
				{
					dsOrder: []int{2, 0, 1},
					suggestions: NodeSuggestions{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 11, Path: "query.me", ParentPath: "query", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "details", DataSourceHash: 11, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
						{TypeName: "Details", FieldName: "forename", DataSourceHash: 11, Path: "query.me.details.forename", ParentPath: "query.me.details", IsRootNode: false, selected: true},
						{TypeName: "Details", FieldName: "surname", DataSourceHash: 22, Path: "query.me.details.surname", ParentPath: "query.me.details", IsRootNode: false, selected: true},
						{TypeName: "Details", FieldName: "middlename", DataSourceHash: 11, Path: "query.me.details.middlename", ParentPath: "query.me.details", IsRootNode: false, selected: true},
					},
				},
				{
					dsOrder: []int{2, 1, 0},
					suggestions: NodeSuggestions{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 22, Path: "query.me", ParentPath: "query", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "details", DataSourceHash: 11, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
						{TypeName: "Details", FieldName: "forename", DataSourceHash: 22, Path: "query.me.details.forename", ParentPath: "query.me.details", IsRootNode: false, selected: true},
						{TypeName: "Details", FieldName: "surname", DataSourceHash: 22, Path: "query.me.details.surname", ParentPath: "query.me.details", IsRootNode: false, selected: true},
						{TypeName: "Details", FieldName: "middlename", DataSourceHash: 11, Path: "query.me.details.middlename", ParentPath: "query.me.details", IsRootNode: false, selected: true},
					},
				},
			},
		},
		{
			Description: "Shareable: should use 2 ds for single field",
			Definition:  shareableDefinition,
			Query: `
				query {
					me {
						details {
							age
						}
					}
				}
			`,
			DataSources: []DataSourceConfiguration{
				shareableDS1,
				shareableDS2,
				shareableDS3,
			},
			ExpectedVariants: []Variant{
				{
					dsOrder: []int{2, 1, 0},
					suggestions: NodeSuggestions{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 22, Path: "query.me", ParentPath: "query", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "details", DataSourceHash: 33, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
						{TypeName: "Details", FieldName: "age", DataSourceHash: 33, Path: "query.me.details.age", ParentPath: "query.me.details", IsRootNode: false, selected: true},
					},
				},
				{
					dsOrder: []int{0, 1, 2},
					suggestions: NodeSuggestions{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 11, Path: "query.me", ParentPath: "query", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "details", DataSourceHash: 33, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
						{TypeName: "Details", FieldName: "age", DataSourceHash: 33, Path: "query.me.details.age", ParentPath: "query.me.details", IsRootNode: false, selected: true},
					},
				},
			},
		},
		{
			Description: "Shareable: should select details from correct ds",
			Definition:  shareableDefinition,
			Query: `
				query {
					me {
						details {
							pets {
								name
							}
						}
					}
				}
			`,
			DataSources: []DataSourceConfiguration{
				shareableDS3,
				shareableDS1,
			},
			ExpectedVariants: []Variant{
				{
					dsOrder: []int{0, 1},
					suggestions: NodeSuggestions{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 11, Path: "query.me", ParentPath: "query", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "details", DataSourceHash: 33, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
						{TypeName: "Details", FieldName: "pets", DataSourceHash: 33, Path: "query.me.details.pets", ParentPath: "query.me.details", IsRootNode: false, selected: true},
						{TypeName: "Pet", FieldName: "name", DataSourceHash: 33, Path: "query.me.details.pets.name", ParentPath: "query.me.details.pets", IsRootNode: false, selected: true},
					},
				},
				{
					dsOrder: []int{1, 0},
					suggestions: NodeSuggestions{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 11, Path: "query.me", ParentPath: "query", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "details", DataSourceHash: 33, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
						{TypeName: "Details", FieldName: "pets", DataSourceHash: 33, Path: "query.me.details.pets", ParentPath: "query.me.details", IsRootNode: false, selected: true},
						{TypeName: "Pet", FieldName: "name", DataSourceHash: 33, Path: "query.me.details.pets.name", ParentPath: "query.me.details.pets", IsRootNode: false, selected: true},
					},
				},
			},
		},
		{
			Description: "Shareable: should use all ds",
			Definition:  shareableDefinition,
			Query: `
				query {
					me {
						details {
							forename
							surname
							middlename
							age
						}
					}
				}
			`,
			DataSources: []DataSourceConfiguration{
				shareableDS1,
				shareableDS2,
				shareableDS3,
			},
			ExpectedVariants: []Variant{
				{
					dsOrder: []int{0, 1, 2},
					suggestions: NodeSuggestions{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 11, Path: "query.me", ParentPath: "query", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "details", DataSourceHash: 11, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "details", DataSourceHash: 33, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
						{TypeName: "Details", FieldName: "forename", DataSourceHash: 11, Path: "query.me.details.forename", ParentPath: "query.me.details", IsRootNode: false, selected: true},
						{TypeName: "Details", FieldName: "surname", DataSourceHash: 22, Path: "query.me.details.surname", ParentPath: "query.me.details", IsRootNode: false, selected: true},
						{TypeName: "Details", FieldName: "middlename", DataSourceHash: 11, Path: "query.me.details.middlename", ParentPath: "query.me.details", IsRootNode: false, selected: true},
						{TypeName: "Details", FieldName: "age", DataSourceHash: 33, Path: "query.me.details.age", ParentPath: "query.me.details", IsRootNode: false, selected: true},
					},
				},
				{
					dsOrder: []int{1, 0, 2},
					suggestions: NodeSuggestions{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 22, Path: "query.me", ParentPath: "query", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "details", DataSourceHash: 11, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "details", DataSourceHash: 33, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
						{TypeName: "Details", FieldName: "forename", DataSourceHash: 22, Path: "query.me.details.forename", ParentPath: "query.me.details", IsRootNode: false, selected: true},
						{TypeName: "Details", FieldName: "surname", DataSourceHash: 22, Path: "query.me.details.surname", ParentPath: "query.me.details", IsRootNode: false, selected: true},
						{TypeName: "Details", FieldName: "middlename", DataSourceHash: 11, Path: "query.me.details.middlename", ParentPath: "query.me.details", IsRootNode: false, selected: true},
						{TypeName: "Details", FieldName: "age", DataSourceHash: 33, Path: "query.me.details.age", ParentPath: "query.me.details", IsRootNode: false, selected: true},
					},
				},
				{
					dsOrder: []int{2, 1, 0},
					suggestions: NodeSuggestions{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 22, Path: "query.me", ParentPath: "query", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "details", DataSourceHash: 33, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "details", DataSourceHash: 11, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
						{TypeName: "Details", FieldName: "forename", DataSourceHash: 22, Path: "query.me.details.forename", ParentPath: "query.me.details", IsRootNode: false, selected: true},
						{TypeName: "Details", FieldName: "surname", DataSourceHash: 22, Path: "query.me.details.surname", ParentPath: "query.me.details", IsRootNode: false, selected: true},
						{TypeName: "Details", FieldName: "middlename", DataSourceHash: 11, Path: "query.me.details.middlename", ParentPath: "query.me.details", IsRootNode: false, selected: true},
						{TypeName: "Details", FieldName: "age", DataSourceHash: 33, Path: "query.me.details.age", ParentPath: "query.me.details", IsRootNode: false, selected: true},
					},
				},
			},
		},
		{
			Description: "Shareable: no root node parent",
			Definition:  conflictingPathsDefinition,
			Query: `
				query {
					user {
						id
						object {
							name
						}
						nested {
							uniqueOne
							uniqueTwo
							nested {
								shared
								uniqueOne
								uniqueTwo
							}
						}
					}
				}
			`,
			DataSources: []DataSourceConfiguration{
				conflictingPaths1,
				conflictingPaths2,
			},
			ExpectedVariants: []Variant{
				{
					dsOrder: []int{0, 1},
					suggestions: NodeSuggestions{
						{TypeName: "Query", FieldName: "user", DataSourceHash: 11, Path: "query.user", ParentPath: "query", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "id", DataSourceHash: 11, Path: "query.user.id", ParentPath: "query.user", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "object", DataSourceHash: 11, Path: "query.user.object", ParentPath: "query.user", IsRootNode: true, selected: true},
						{TypeName: "Object", FieldName: "name", DataSourceHash: 11, Path: "query.user.object.name", ParentPath: "query.user.object", IsRootNode: false, selected: true},
						{TypeName: "User", FieldName: "nested", DataSourceHash: 11, Path: "query.user.nested", ParentPath: "query.user", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "nested", DataSourceHash: 22, Path: "query.user.nested", ParentPath: "query.user", IsRootNode: true, selected: true},
						{TypeName: "NestedOne", FieldName: "uniqueOne", DataSourceHash: 11, Path: "query.user.nested.uniqueOne", ParentPath: "query.user.nested", IsRootNode: false, selected: true},
						{TypeName: "NestedOne", FieldName: "uniqueTwo", DataSourceHash: 22, Path: "query.user.nested.uniqueTwo", ParentPath: "query.user.nested", IsRootNode: false, selected: true},
						{TypeName: "NestedOne", FieldName: "nested", DataSourceHash: 11, Path: "query.user.nested.nested", ParentPath: "query.user.nested", IsRootNode: false, selected: true},
						{TypeName: "NestedOne", FieldName: "nested", DataSourceHash: 22, Path: "query.user.nested.nested", ParentPath: "query.user.nested", IsRootNode: false, selected: true},
						{TypeName: "NestedTwo", FieldName: "shared", DataSourceHash: 11, Path: "query.user.nested.nested.shared", ParentPath: "query.user.nested.nested", IsRootNode: false, selected: true},
						{TypeName: "NestedTwo", FieldName: "uniqueOne", DataSourceHash: 11, Path: "query.user.nested.nested.uniqueOne", ParentPath: "query.user.nested.nested", IsRootNode: false, selected: true},
						{TypeName: "NestedTwo", FieldName: "uniqueTwo", DataSourceHash: 22, Path: "query.user.nested.nested.uniqueTwo", ParentPath: "query.user.nested.nested", IsRootNode: false, selected: true},
					},
				},
				{
					dsOrder: []int{1, 0},
					suggestions: NodeSuggestions{
						{TypeName: "Query", FieldName: "user", DataSourceHash: 11, Path: "query.user", ParentPath: "query", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "id", DataSourceHash: 11, Path: "query.user.id", ParentPath: "query.user", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "object", DataSourceHash: 11, Path: "query.user.object", ParentPath: "query.user", IsRootNode: true, selected: true},
						{TypeName: "Object", FieldName: "name", DataSourceHash: 11, Path: "query.user.object.name", ParentPath: "query.user.object", IsRootNode: false, selected: true},
						{TypeName: "User", FieldName: "nested", DataSourceHash: 22, Path: "query.user.nested", ParentPath: "query.user", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "nested", DataSourceHash: 11, Path: "query.user.nested", ParentPath: "query.user", IsRootNode: true, selected: true},
						{TypeName: "NestedOne", FieldName: "uniqueOne", DataSourceHash: 11, Path: "query.user.nested.uniqueOne", ParentPath: "query.user.nested", IsRootNode: false, selected: true},
						{TypeName: "NestedOne", FieldName: "uniqueTwo", DataSourceHash: 22, Path: "query.user.nested.uniqueTwo", ParentPath: "query.user.nested", IsRootNode: false, selected: true},
						{TypeName: "NestedOne", FieldName: "nested", DataSourceHash: 22, Path: "query.user.nested.nested", ParentPath: "query.user.nested", IsRootNode: false, selected: true},
						{TypeName: "NestedOne", FieldName: "nested", DataSourceHash: 11, Path: "query.user.nested.nested", ParentPath: "query.user.nested", IsRootNode: false, selected: true},
						{TypeName: "NestedTwo", FieldName: "shared", DataSourceHash: 22, Path: "query.user.nested.nested.shared", ParentPath: "query.user.nested.nested", IsRootNode: false, selected: true},
						{TypeName: "NestedTwo", FieldName: "uniqueOne", DataSourceHash: 11, Path: "query.user.nested.nested.uniqueOne", ParentPath: "query.user.nested.nested", IsRootNode: false, selected: true},
						{TypeName: "NestedTwo", FieldName: "uniqueTwo", DataSourceHash: 22, Path: "query.user.nested.nested.uniqueTwo", ParentPath: "query.user.nested.nested", IsRootNode: false, selected: true},
					},
				},
			},
		},
		{
			Description: "Shareable: conflicting paths with relay style pagination over shareable child nodes",
			Definition:  conflictingShareablePathsRelayStyleDefinition,
			Query: `
				query {
					users {
						edges {
							node {
								id
								firstName
								lastName
								address {
									street
								}
							}
						}
					}
				}
			`,
			DataSources: []DataSourceConfiguration{
				conflictingShareablePathsRelayStyle1,
				conflictingShareablePathsRelayStyle2,
			},
			ExpectedVariants: []Variant{
				{
					dsOrder: []int{0, 1},
					suggestions: NodeSuggestions{
						{TypeName: "Query", FieldName: "users", DataSourceHash: 11, Path: "query.users", ParentPath: "query", IsRootNode: true, selected: true},
						{TypeName: "PaginatedUser", FieldName: "edges", DataSourceHash: 11, Path: "query.users.edges", ParentPath: "query.users", IsRootNode: false, selected: true},
						{TypeName: "UserToEdge", FieldName: "node", DataSourceHash: 11, Path: "query.users.edges.node", ParentPath: "query.users.edges", IsRootNode: false, selected: true},
						{TypeName: "User", FieldName: "id", DataSourceHash: 11, Path: "query.users.edges.node.id", ParentPath: "query.users.edges.node", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "firstName", DataSourceHash: 11, Path: "query.users.edges.node.firstName", ParentPath: "query.users.edges.node", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "lastName", DataSourceHash: 11, Path: "query.users.edges.node.lastName", ParentPath: "query.users.edges.node", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "address", DataSourceHash: 22, Path: "query.users.edges.node.address", ParentPath: "query.users.edges.node", IsRootNode: true, selected: true},
						{TypeName: "Address", FieldName: "street", DataSourceHash: 22, Path: "query.users.edges.node.address.street", ParentPath: "query.users.edges.node.address", IsRootNode: true, selected: true},
					},
				},
				{
					dsOrder: []int{1, 0},
					suggestions: NodeSuggestions{
						{TypeName: "Query", FieldName: "users", DataSourceHash: 11, Path: "query.users", ParentPath: "query", IsRootNode: true, selected: true},
						{TypeName: "PaginatedUser", FieldName: "edges", DataSourceHash: 11, Path: "query.users.edges", ParentPath: "query.users", IsRootNode: false, selected: true},
						{TypeName: "UserToEdge", FieldName: "node", DataSourceHash: 11, Path: "query.users.edges.node", ParentPath: "query.users.edges", IsRootNode: false, selected: true},
						{TypeName: "User", FieldName: "id", DataSourceHash: 11, Path: "query.users.edges.node.id", ParentPath: "query.users.edges.node", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "firstName", DataSourceHash: 11, Path: "query.users.edges.node.firstName", ParentPath: "query.users.edges.node", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "lastName", DataSourceHash: 11, Path: "query.users.edges.node.lastName", ParentPath: "query.users.edges.node", IsRootNode: true, selected: true},
						{TypeName: "User", FieldName: "address", DataSourceHash: 22, Path: "query.users.edges.node.address", ParentPath: "query.users.edges.node", IsRootNode: true, selected: true},
						{TypeName: "Address", FieldName: "street", DataSourceHash: 22, Path: "query.users.edges.node.address.street", ParentPath: "query.users.edges.node.address", IsRootNode: true, selected: true},
					},
				},
			},
		},
	}

	run := func(t *testing.T, Definition, Query string, DataSources []DataSourceConfiguration, expected NodeSuggestions) {
		t.Helper()

		definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(Definition)
		operation := unsafeparser.ParseGraphqlDocumentString(Query)
		report := operationreport.Report{}

		astvalidation.DefaultOperationValidator().Validate(&operation, &definition, &report)
		if report.HasErrors() {
			t.Fatal(report.Error())
		}

		dsFilter := NewDataSourceFilter(&operation, &definition, &report)
		if report.HasErrors() {
			t.Fatal(report.Error())
		}

		planned := dsFilter.findBestDataSourceSet(DataSources, nil)
		if report.HasErrors() {
			t.Fatal(report.Error())
		}

		if !assert.Equal(t, expected, planned) {
			expected, _ := json.MarshalIndent(expected, "", "  ")
			planned, _ := json.MarshalIndent(planned, "", "  ")

			assert.Equal(t, string(expected), string(planned))
		}
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.Description, func(t *testing.T) {

			if tc.ExpectedSuggestions != nil {
				run(t, tc.Definition, tc.Query, shuffleDS(tc.DataSources), tc.ExpectedSuggestions)
				return
			}

			for i, variant := range tc.ExpectedVariants {
				variant := variant
				t.Run(fmt.Sprintf("Variant: %d", i), func(t *testing.T) {
					run(t, tc.Definition, tc.Query, orderDS(tc.DataSources, variant.dsOrder), variant.suggestions)
				})
			}
		})
	}
}

// shuffleDS randomizes the order of the data sources
// to ensure that the order doesn't matter
func shuffleDS(dataSources []DataSourceConfiguration) []DataSourceConfiguration {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	rnd.Shuffle(len(dataSources), func(i, j int) {
		dataSources[i], dataSources[j] = dataSources[j], dataSources[i]
	})

	return dataSources
}

func orderDS(dataSources []DataSourceConfiguration, order []int) (out []DataSourceConfiguration) {
	out = make([]DataSourceConfiguration, 0, len(dataSources))

	for _, i := range order {
		out = append(out, dataSources[i])
	}

	return out
}

const shareableDefinition = `
	type User {
		id: ID!
		details: Details!
	}

	type Details {
		forename: String!
		surname: String!
		middlename: String!
		age: Int!
		pets: [Pet!]
	}

	type Pet {
		name: String!
	}

	type Query {
		me: User
	}`

const shareableDS1Schema = `
	type User @key(fields: "id") {
		id: ID!
		details: Details! @shareable
	}
	
	type Details {
		forename: String! @shareable
		middlename: String!
	}
	
	type Query {
		me: User
	}
`

var shareableDS1 = dsb().Hash(11).Schema(shareableDS1Schema).
	RootNode("Query", "me").
	RootNode("User", "id", "details").
	ChildNode("Details", "forename", "middlename").
	DS()

const shareableDS2Schema = `
	type User @key(fields: "id") {
		id: ID!
		details: Details! @shareable
	}

	type Details {
		forename: String! @shareable
		surname: String!
	}

	type Query {
		me: User
	}
`

var shareableDS2 = dsb().Hash(22).Schema(shareableDS2Schema).
	RootNode("Query", "me").
	RootNode("User", "id", "details").
	ChildNode("Details", "forename", "surname").
	DS()

const shareableDS3Schema = `
	type User @key(fields: "id") {
		id: ID!
		details: Details! @shareable
	}

	type Details {
		age: Int!
		pets: [Pet!]
	}

	type Pet {
		name: String!
	}
`

var shareableDS3 = dsb().Hash(33).Schema(shareableDS3Schema).
	RootNode("User", "id", "details").
	ChildNode("Details", "age", "pets").
	ChildNode("Pet", "name").
	DS()

const conflictingPaths1Schema = `
	type Query {
		user: User!
	}

	type User @key(fields: "id") {
		id: ID!
		nested: NestedOne!
	}

	type NestedOne {
		uniqueOne: String!
		nested: NestedTwo!
	}

	type NestedTwo {
		shared: String!
		uniqueOne: Int!
	}
`

var conflictingPaths1 = dsb().Hash(11).Schema(conflictingPaths1Schema).
	RootNode("Query", "user").RootNode("User", "id", "nested", "object").
	ChildNode("Object", "name").
	ChildNode("NestedOne", "uniqueOne", "nested").
	ChildNode("NestedTwo", "shared", "uniqueOne").
	DS()

const conflictingPaths2Schema = `
	type User @key(fields: "id") {
		id: ID!
		nested: NestedOne!
	}

	type NestedOne {
		uniqueTwo: String!
		nested: NestedTwo!
	}

	type NestedTwo {
		shared: String!
		uniqueTwo: Int!
	}
`

var conflictingPaths2 = dsb().Hash(22).Schema(conflictingPaths2Schema).
	RootNode("User", "id", "nested").
	ChildNode("NestedOne", "uniqueTwo", "nested").
	ChildNode("NestedTwo", "shared", "uniqueTwo").
	DS()

var conflictingPathsDefinition = `
	type Query {
		user: User!
	}
	
	type User {
		id: ID!
		nested: NestedOne!
		object: Object!
	}

	type Object {
		name: String!
	}
	
	type NestedOne {
		uniqueOne: String!
		uniqueTwo: String!
		nested: NestedTwo!
	}
	
	type NestedTwo {
		shared: String!
		uniqueOne: Int!
		uniqueTwo: Int!
	}
`

const conflictingShareablePathsRelayStyleSchema1 = `
	type User @key(fields: "id") {
		id: ID!
		firstName: String!
		lastName: String!
	}
	
	type PaginatedUser @shareable {
		edges: [UserToEdge!]
		nodes: [User!]
		totalCount: Int!
		hasNextPage: Boolean!
	}
	
	type UserToEdge @shareable {
		node: User!
	}
	
	type Query {
		users: PaginatedUser!
	}`

var conflictingShareablePathsRelayStyle1 = dsb().Hash(11).Schema(conflictingShareablePathsRelayStyleSchema1).
	RootNode("Query", "users").
	RootNode("User", "id", "firstName", "lastName").
	ChildNode("PaginatedUser", "edges", "nodes", "totalCount", "hasNextPage").
	ChildNode("UserToEdge", "node").
	DS()

const conflictingShareablePathsRelayStyleSchema2 = `
	type User @key(fields: "id") {
		id: ID!
		address: Address!
	}
	
	type PaginatedUser @shareable {
		edges: [UserToEdge!]
		nodes: [User!]
		totalCount: Int!
		hasNextPage: Boolean!
	}
	
	type UserToEdge @shareable {
		node: User!
	}

	type Address @key(fields: "id") {
		street: String!
		residents: PaginatedUser!
	}

	type Query {
		address(id: ID!): Address!
	}`

var conflictingShareablePathsRelayStyle2 = dsb().Hash(22).Schema(conflictingShareablePathsRelayStyleSchema2).
	RootNode("Query", "address").
	RootNode("User", "id", "address").
	RootNode("Address", "street", "residents").
	ChildNode("PaginatedUser", "edges", "nodes", "totalCount", "hasNextPage").
	ChildNode("UserToEdge", "node").
	DS()

var conflictingShareablePathsRelayStyleDefinition = `
	type User {
		id: ID!
		firstName: String!
		lastName: String!
		address: Address!
	}
	
	type PaginatedUser {
		edges: [UserToEdge!]
		nodes: [User!]
		totalCount: Int!
		hasNextPage: Boolean!
	}
	
	type UserToEdge {
		node: User!
	}

	type Address {
		street: String!
		residents: PaginatedUser!
	}
	
	type Query {
		users: PaginatedUser!
		address(id: ID!): Address!
	}`
