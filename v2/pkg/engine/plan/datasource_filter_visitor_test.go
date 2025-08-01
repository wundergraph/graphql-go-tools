package plan

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/kylelemons/godebug/pretty"
	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/testing/permutations"
)

type dsBuilder struct {
	ds       *dataSourceConfiguration[any]
	behavior *DataSourcePlanningBehavior
}

func dsb() *dsBuilder {
	return &dsBuilder{ds: &dataSourceConfiguration[any]{DataSourceMetadata: &DataSourceMetadata{}}}
}

func (b *dsBuilder) RootNode(typeName string, fieldNames ...string) *dsBuilder {
	b.ds.RootNodes = append(b.ds.RootNodes, TypeField{TypeName: typeName, FieldNames: fieldNames})
	return b
}

func (b *dsBuilder) AddRootNodeExternalFieldNames(typeName string, fieldNames ...string) *dsBuilder {
	for i := range b.ds.RootNodes {
		if b.ds.RootNodes[i].TypeName == typeName {
			b.ds.RootNodes[i].ExternalFieldNames = fieldNames
			return b
		}
	}
	return b
}

func (b *dsBuilder) ChildNode(typeName string, fieldNames ...string) *dsBuilder {
	b.ds.ChildNodes = append(b.ds.ChildNodes, TypeField{TypeName: typeName, FieldNames: fieldNames})
	return b
}

func (b *dsBuilder) AddChildNodeExternalFieldNames(typeName string, fieldNames ...string) *dsBuilder {
	for i := range b.ds.ChildNodes {
		if b.ds.ChildNodes[i].TypeName == typeName {
			b.ds.ChildNodes[i].ExternalFieldNames = fieldNames
			return b
		}
	}
	return b
}

func (b *dsBuilder) Schema(schema string) *dsBuilder {
	def := unsafeparser.ParseGraphqlDocumentString(schema)
	b.ds.factory = &FakeFactory[any]{
		upstreamSchema: &def,
		behavior:       b.behavior,
	}

	return b
}

func (b *dsBuilder) KeysMetadata(keys FederationFieldConfigurations) *dsBuilder {
	b.ds.FederationMetaData.Keys = keys
	return b
}

func (b *dsBuilder) WithMetadata(f func(data *FederationMetaData)) *dsBuilder {
	f(&b.ds.FederationMetaData)
	return b
}

func (b *dsBuilder) Hash(hash DSHash) *dsBuilder {
	b.ds.hash = hash
	return b
}

func (b *dsBuilder) Id(id string) *dsBuilder {
	b.ds.id = id
	return b
}

func (b *dsBuilder) DS() DataSource {
	if err := b.ds.DataSourceMetadata.Init(); err != nil {
		panic(err)
	}
	return b.ds
}

func (b *dsBuilder) WithBehavior(behavior DataSourcePlanningBehavior) *dsBuilder {
	b.behavior = &behavior
	return b
}

func strptr(s string) *string { return &s }

func newNodeSuggestions(nodes []NodeSuggestion) *NodeSuggestions {
	items := make([]*NodeSuggestion, 0, len(nodes))
	for i := range nodes {
		items = append(items, &nodes[i])
	}
	return &NodeSuggestions{items: items}
}

func TestFindBestDataSourceSet(t *testing.T) {
	type Variant struct {
		dsOrder     []int
		suggestions *NodeSuggestions
	}

	type TestCase struct {
		Description         string
		Definition          string
		Query               string
		DataSources         []DataSource
		ExpectedVariants    []Variant
		ExpectedSuggestions *NodeSuggestions
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
			DataSources: []DataSource{
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
			ExpectedSuggestions: newNodeSuggestions([]NodeSuggestion{
				{TypeName: "Query", FieldName: "provider", DataSourceHash: 11, Path: "query.provider", ParentPath: "query", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
				{TypeName: "AccountProvider", FieldName: "accounts", DataSourceHash: 11, Path: "query.provider.accounts", ParentPath: "query.provider", Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
				{TypeName: "User", FieldName: "name", DataSourceHash: 22, Path: "query.provider.accounts.$0User.name", ParentPath: "query.provider.accounts.$0User", onFragment: true, parentPathWithoutFragment: "query.provider.accounts", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
			}),
		},
		{
			Description: "Choose users from first data source and name from the second with __typename",
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
							__typename
							... on User {
								name
							}
						}
					}
				}
			`,
			DataSources: []DataSource{
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
			ExpectedSuggestions: newNodeSuggestions([]NodeSuggestion{
				{TypeName: "Query", FieldName: "provider", DataSourceHash: 11, Path: "query.provider", ParentPath: "query", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
				{TypeName: "AccountProvider", FieldName: "accounts", DataSourceHash: 11, Path: "query.provider.accounts", ParentPath: "query.provider", Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
				{TypeName: "User", FieldName: "name", DataSourceHash: 22, Path: "query.provider.accounts.$0User.name", ParentPath: "query.provider.accounts.$0User", onFragment: true, parentPathWithoutFragment: "query.provider.accounts", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
				{TypeName: "Account", FieldName: "__typename", DataSourceHash: 11, Path: "query.provider.accounts.__typename", ParentPath: "query.provider.accounts", IsRootNode: false, isTypeName: true, possibleTypeNames: []string{"User"}, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
			}),
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
			DataSources: []DataSource{
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
			ExpectedSuggestions: newNodeSuggestions([]NodeSuggestion{
				{TypeName: "Query", FieldName: "user", DataSourceHash: 11, Path: "query.user", ParentPath: "query", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
				{TypeName: "User", FieldName: "id", DataSourceHash: 11, Path: "query.user.id", ParentPath: "query.user", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
				{TypeName: "User", FieldName: "name", DataSourceHash: 22, Path: "query.user.name", ParentPath: "query.user", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
				{TypeName: "User", FieldName: "surname", DataSourceHash: 22, Path: "query.user.surname", ParentPath: "query.user", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
			}),
		},
		{
			Description: "do not select external node",
			Definition: `
				type Query {
					wrapper: Wrapper
				}
				type Wrapper {
					id: Int
					user: User
				}
				type User {
					id: Int
					name: String
					surname: String
				}	
			`,
			Query: `
				query {
					wrapper {
						user {
							name
							surname
						}
					}
				}
			`,
			DataSources: []DataSource{
				dsb().Hash(11).Schema(`
					type Query {
						wrapper: Wrapper
					}

					type Wrapper @key(fields: "id") {
						id: Int
					}
				`).RootNode("Query", "wrapper").
					RootNode("Wrapper", "id").DS(),
				dsb().Hash(22).Schema(`
					type Query {
						wrapper2: Wrapper
					}

					type Wrapper {
						id: Int
						user: User @external
					}

					type User @key(fields: "id") {
						id: Int
						name: String
					}
				`).RootNode("Query", "wrapper2").
					RootNode("Wrapper", "id").
					AddRootNodeExternalFieldNames("Wrapper", "user").
					// artificial child node - in reality it could be hidden under interface
					// which is always a child node
					ChildNode("User", "id", "name").
					DS(),
				dsb().Hash(33).Schema(`
					type Wrapper {
						id: Int
						user: User
					}

					type User @key(fields: "id") {
						id: Int
						surname: String
					}
				`).RootNode("User", "id", "surname").
					RootNode("Wrapper", "user").DS(),
			},
			ExpectedSuggestions: newNodeSuggestions([]NodeSuggestion{
				{TypeName: "Query", FieldName: "wrapper", DataSourceHash: 11, Path: "query.wrapper", ParentPath: "query", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
				{TypeName: "Wrapper", FieldName: "user", DataSourceHash: 33, Path: "query.wrapper.user", ParentPath: "query.wrapper", IsRootNode: true, IsExternal: false, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected child"}},
				{TypeName: "User", FieldName: "name", DataSourceHash: 22, Path: "query.wrapper.user.name", ParentPath: "query.wrapper.user", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
				{TypeName: "User", FieldName: "surname", DataSourceHash: 33, Path: "query.wrapper.user.surname", ParentPath: "query.wrapper.user", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
			}),
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
			DataSources: []DataSource{
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
			ExpectedSuggestions: newNodeSuggestions([]NodeSuggestion{
				{TypeName: "Query", FieldName: "user", DataSourceHash: 11, Path: "query.user", ParentPath: "query", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
				{TypeName: "User", FieldName: "age", DataSourceHash: 11, Path: "query.user.age", ParentPath: "query.user", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
				{TypeName: "User", FieldName: "name", DataSourceHash: 33, Path: "query.user.name", ParentPath: "query.user", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected sibling"}},
				{TypeName: "User", FieldName: "surname", DataSourceHash: 33, Path: "query.user.surname", ParentPath: "query.user", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
			}),
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
			DataSources: []DataSource{
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
			ExpectedSuggestions: newNodeSuggestions([]NodeSuggestion{
				{TypeName: "Query", FieldName: "user", DataSourceHash: 22, Path: "query.user", ParentPath: "query", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected child"}},
				{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.user.details", ParentPath: "query.user", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
				{TypeName: "Details", FieldName: "address", DataSourceHash: 22, Path: "query.user.details.address", ParentPath: "query.user.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
				{TypeName: "Address", FieldName: "lines", DataSourceHash: 22, Path: "query.user.details.address.lines", ParentPath: "query.user.details.address", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
				{TypeName: "Lines", FieldName: "id", DataSourceHash: 22, Path: "query.user.details.address.lines.id", ParentPath: "query.user.details.address.lines", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
				{TypeName: "Lines", FieldName: "line1", DataSourceHash: 44, Path: "query.user.details.address.lines.line1", ParentPath: "query.user.details.address.lines", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected sibling"}},
				{TypeName: "Lines", FieldName: "line2", DataSourceHash: 44, Path: "query.user.details.address.lines.line2", ParentPath: "query.user.details.address.lines", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
				{TypeName: "Address", FieldName: "name", DataSourceHash: 22, Path: "query.user.details.address.name", ParentPath: "query.user.details.address", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
				{TypeName: "Details", FieldName: "age", DataSourceHash: 22, Path: "query.user.details.age", ParentPath: "query.user.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
				{TypeName: "User", FieldName: "name", DataSourceHash: 22, Path: "query.user.name", ParentPath: "query.user", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
			}),
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
			DataSources: []DataSource{
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
					suggestions: newNodeSuggestions([]NodeSuggestion{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 11, Path: "query.me", ParentPath: "query", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "User", FieldName: "details", DataSourceHash: 11, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "Details", FieldName: "name", DataSourceHash: 11, Path: "query.me.details.name", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "Details", FieldName: "surname", DataSourceHash: 22, Path: "query.me.details.surname", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
					}),
				},
				{
					dsOrder: []int{1, 0},
					suggestions: newNodeSuggestions([]NodeSuggestion{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 11, Path: "query.me", ParentPath: "query", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "User", FieldName: "details", DataSourceHash: 11, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "Details", FieldName: "name", DataSourceHash: 11, Path: "query.me.details.name", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "Details", FieldName: "surname", DataSourceHash: 22, Path: "query.me.details.surname", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
					}),
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
			DataSources: []DataSource{
				shareableDS1,
				shareableDS2,
				shareableDS3,
			},
			ExpectedVariants: []Variant{
				{
					dsOrder: []int{0, 1, 2},
					suggestions: newNodeSuggestions([]NodeSuggestion{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 11, Path: "query.me", ParentPath: "query", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage3: first available root node"}},
						{TypeName: "User", FieldName: "details", DataSourceHash: 11, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
						{TypeName: "Details", FieldName: "forename", DataSourceHash: 11, Path: "query.me.details.forename", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
					}),
				},
				{
					dsOrder: []int{2, 1, 0},
					suggestions: newNodeSuggestions([]NodeSuggestion{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 22, Path: "query.me", ParentPath: "query", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage3: first available root node"}},
						{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
						{TypeName: "Details", FieldName: "forename", DataSourceHash: 22, Path: "query.me.details.forename", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
					}),
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
			DataSources: []DataSource{
				shareableDS1,
				shareableDS2,
				shareableDS3,
			},
			ExpectedSuggestions: newNodeSuggestions([]NodeSuggestion{
				{TypeName: "Query", FieldName: "me", DataSourceHash: 22, Path: "query.me", ParentPath: "query", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected child"}},
				{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
				{TypeName: "Details", FieldName: "forename", DataSourceHash: 22, Path: "query.me.details.forename", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
				{TypeName: "Details", FieldName: "surname", DataSourceHash: 22, Path: "query.me.details.surname", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
			}),
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
			DataSources: []DataSource{
				shareableDS1,
				shareableDS2,
				shareableDS3,
			},
			ExpectedVariants: []Variant{
				{
					dsOrder: []int{1, 0, 2},
					suggestions: newNodeSuggestions([]NodeSuggestion{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 22, Path: "query.me", ParentPath: "query", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected child"}},
						{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "User", FieldName: "details", DataSourceHash: 11, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "Details", FieldName: "forename", DataSourceHash: 22, Path: "query.me.details.forename", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
						{TypeName: "Details", FieldName: "middlename", DataSourceHash: 11, Path: "query.me.details.middlename", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "Details", FieldName: "surname", DataSourceHash: 22, Path: "query.me.details.surname", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
					}),
				},
				{
					dsOrder: []int{2, 0, 1},
					suggestions: newNodeSuggestions([]NodeSuggestion{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 11, Path: "query.me", ParentPath: "query", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected child"}},
						{TypeName: "User", FieldName: "details", DataSourceHash: 11, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "Details", FieldName: "forename", DataSourceHash: 11, Path: "query.me.details.forename", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
						{TypeName: "Details", FieldName: "middlename", DataSourceHash: 11, Path: "query.me.details.middlename", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "Details", FieldName: "surname", DataSourceHash: 22, Path: "query.me.details.surname", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
					}),
				},
				{
					dsOrder: []int{2, 1, 0},
					suggestions: newNodeSuggestions([]NodeSuggestion{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 22, Path: "query.me", ParentPath: "query", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected child"}},
						{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "User", FieldName: "details", DataSourceHash: 11, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "Details", FieldName: "forename", DataSourceHash: 22, Path: "query.me.details.forename", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
						{TypeName: "Details", FieldName: "middlename", DataSourceHash: 11, Path: "query.me.details.middlename", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "Details", FieldName: "surname", DataSourceHash: 22, Path: "query.me.details.surname", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
					}),
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
			DataSources: []DataSource{
				shareableDS1,
				shareableDS2,
				shareableDS3,
			},
			ExpectedVariants: []Variant{
				{
					dsOrder: []int{2, 1, 0},
					suggestions: newNodeSuggestions([]NodeSuggestion{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 22, Path: "query.me", ParentPath: "query", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage3: first available root node"}},
						{TypeName: "User", FieldName: "details", DataSourceHash: 33, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "Details", FieldName: "age", DataSourceHash: 33, Path: "query.me.details.age", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
					}),
				},
				{
					dsOrder: []int{0, 1, 2},
					suggestions: newNodeSuggestions([]NodeSuggestion{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 11, Path: "query.me", ParentPath: "query", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage3: first available root node"}},
						{TypeName: "User", FieldName: "details", DataSourceHash: 33, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "Details", FieldName: "age", DataSourceHash: 33, Path: "query.me.details.age", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
					}),
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
			DataSources: []DataSource{
				shareableDS3,
				shareableDS1,
			},
			ExpectedVariants: []Variant{
				{
					dsOrder: []int{0, 1},
					suggestions: newNodeSuggestions([]NodeSuggestion{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 11, Path: "query.me", ParentPath: "query", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "User", FieldName: "details", DataSourceHash: 33, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "Details", FieldName: "pets", DataSourceHash: 33, Path: "query.me.details.pets", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "Pet", FieldName: "name", DataSourceHash: 33, Path: "query.me.details.pets.name", ParentPath: "query.me.details.pets", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
					}),
				},
				{
					dsOrder: []int{1, 0},
					suggestions: newNodeSuggestions([]NodeSuggestion{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 11, Path: "query.me", ParentPath: "query", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "User", FieldName: "details", DataSourceHash: 33, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "Details", FieldName: "pets", DataSourceHash: 33, Path: "query.me.details.pets", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "Pet", FieldName: "name", DataSourceHash: 33, Path: "query.me.details.pets.name", ParentPath: "query.me.details.pets", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
					}),
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
			DataSources: []DataSource{
				shareableDS1,
				shareableDS2,
				shareableDS3,
			},
			ExpectedVariants: []Variant{
				{
					dsOrder: []int{0, 1, 2},
					suggestions: newNodeSuggestions([]NodeSuggestion{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 11, Path: "query.me", ParentPath: "query", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected child"}},
						{TypeName: "User", FieldName: "details", DataSourceHash: 11, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "User", FieldName: "details", DataSourceHash: 33, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "Details", FieldName: "age", DataSourceHash: 33, Path: "query.me.details.age", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "Details", FieldName: "forename", DataSourceHash: 11, Path: "query.me.details.forename", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
						{TypeName: "Details", FieldName: "middlename", DataSourceHash: 11, Path: "query.me.details.middlename", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "Details", FieldName: "surname", DataSourceHash: 22, Path: "query.me.details.surname", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
					}),
				},
				{
					dsOrder: []int{1, 0, 2},
					suggestions: newNodeSuggestions([]NodeSuggestion{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 22, Path: "query.me", ParentPath: "query", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected child"}},
						{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "User", FieldName: "details", DataSourceHash: 11, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "User", FieldName: "details", DataSourceHash: 33, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "Details", FieldName: "age", DataSourceHash: 33, Path: "query.me.details.age", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "Details", FieldName: "forename", DataSourceHash: 22, Path: "query.me.details.forename", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
						{TypeName: "Details", FieldName: "middlename", DataSourceHash: 11, Path: "query.me.details.middlename", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "Details", FieldName: "surname", DataSourceHash: 22, Path: "query.me.details.surname", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
					}),
				},
				{
					dsOrder: []int{2, 1, 0},
					suggestions: newNodeSuggestions([]NodeSuggestion{
						{TypeName: "Query", FieldName: "me", DataSourceHash: 22, Path: "query.me", ParentPath: "query", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected child"}},
						{TypeName: "User", FieldName: "details", DataSourceHash: 33, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "User", FieldName: "details", DataSourceHash: 11, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "Details", FieldName: "age", DataSourceHash: 33, Path: "query.me.details.age", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "Details", FieldName: "forename", DataSourceHash: 22, Path: "query.me.details.forename", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
						{TypeName: "Details", FieldName: "middlename", DataSourceHash: 11, Path: "query.me.details.middlename", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "Details", FieldName: "surname", DataSourceHash: 22, Path: "query.me.details.surname", ParentPath: "query.me.details", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
					}),
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
			DataSources: []DataSource{
				conflictingPaths1,
				conflictingPaths2,
			},
			ExpectedVariants: []Variant{
				{
					dsOrder: []int{0, 1},
					suggestions: newNodeSuggestions([]NodeSuggestion{
						{TypeName: "Query", FieldName: "user", DataSourceHash: 11, Path: "query.user", ParentPath: "query", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "User", FieldName: "id", DataSourceHash: 11, Path: "query.user.id", ParentPath: "query.user", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
						{TypeName: "User", FieldName: "nested", DataSourceHash: 11, Path: "query.user.nested", ParentPath: "query.user", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "User", FieldName: "nested", DataSourceHash: 22, Path: "query.user.nested", ParentPath: "query.user", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "NestedOne", FieldName: "nested", DataSourceHash: 11, Path: "query.user.nested.nested", ParentPath: "query.user.nested", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "NestedOne", FieldName: "nested", DataSourceHash: 22, Path: "query.user.nested.nested", ParentPath: "query.user.nested", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "NestedTwo", FieldName: "shared", DataSourceHash: 11, Path: "query.user.nested.nested.shared", ParentPath: "query.user.nested.nested", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
						{TypeName: "NestedTwo", FieldName: "uniqueOne", DataSourceHash: 11, Path: "query.user.nested.nested.uniqueOne", ParentPath: "query.user.nested.nested", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "NestedTwo", FieldName: "uniqueTwo", DataSourceHash: 22, Path: "query.user.nested.nested.uniqueTwo", ParentPath: "query.user.nested.nested", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "NestedOne", FieldName: "uniqueOne", DataSourceHash: 11, Path: "query.user.nested.uniqueOne", ParentPath: "query.user.nested", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "NestedOne", FieldName: "uniqueTwo", DataSourceHash: 22, Path: "query.user.nested.uniqueTwo", ParentPath: "query.user.nested", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "User", FieldName: "object", DataSourceHash: 11, Path: "query.user.object", ParentPath: "query.user", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "Object", FieldName: "name", DataSourceHash: 11, Path: "query.user.object.name", ParentPath: "query.user.object", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
					}),
				},
				{
					dsOrder: []int{1, 0},
					suggestions: newNodeSuggestions([]NodeSuggestion{
						{TypeName: "Query", FieldName: "user", DataSourceHash: 11, Path: "query.user", ParentPath: "query", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "User", FieldName: "id", DataSourceHash: 11, Path: "query.user.id", ParentPath: "query.user", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
						{TypeName: "User", FieldName: "nested", DataSourceHash: 11, Path: "query.user.nested", ParentPath: "query.user", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "User", FieldName: "nested", DataSourceHash: 22, Path: "query.user.nested", ParentPath: "query.user", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "NestedOne", FieldName: "nested", DataSourceHash: 22, Path: "query.user.nested.nested", ParentPath: "query.user.nested", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "NestedOne", FieldName: "nested", DataSourceHash: 11, Path: "query.user.nested.nested", ParentPath: "query.user.nested", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: same source parent of unique node"}},
						{TypeName: "NestedTwo", FieldName: "shared", DataSourceHash: 22, Path: "query.user.nested.nested.shared", ParentPath: "query.user.nested.nested", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
						{TypeName: "NestedTwo", FieldName: "uniqueOne", DataSourceHash: 11, Path: "query.user.nested.nested.uniqueOne", ParentPath: "query.user.nested.nested", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "NestedTwo", FieldName: "uniqueTwo", DataSourceHash: 22, Path: "query.user.nested.nested.uniqueTwo", ParentPath: "query.user.nested.nested", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "NestedOne", FieldName: "uniqueOne", DataSourceHash: 11, Path: "query.user.nested.uniqueOne", ParentPath: "query.user.nested", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "NestedOne", FieldName: "uniqueTwo", DataSourceHash: 22, Path: "query.user.nested.uniqueTwo", ParentPath: "query.user.nested", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "User", FieldName: "object", DataSourceHash: 11, Path: "query.user.object", ParentPath: "query.user", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "Object", FieldName: "name", DataSourceHash: 11, Path: "query.user.object.name", ParentPath: "query.user.object", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage1: unique"}},
					}),
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
			DataSources: []DataSource{
				conflictingShareablePathsRelayStyle1,
				conflictingShareablePathsRelayStyle2,
			},
			ExpectedVariants: []Variant{
				{
					dsOrder: []int{0, 1},
					suggestions: newNodeSuggestions([]NodeSuggestion{
						{TypeName: "Query", FieldName: "users", DataSourceHash: 11, Path: "query.users", ParentPath: "query", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "PaginatedUser", FieldName: "edges", DataSourceHash: 11, Path: "query.users.edges", ParentPath: "query.users", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
						{TypeName: "UserToEdge", FieldName: "node", DataSourceHash: 11, Path: "query.users.edges.node", ParentPath: "query.users.edges", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
						{TypeName: "User", FieldName: "address", DataSourceHash: 22, Path: "query.users.edges.node.address", ParentPath: "query.users.edges.node", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "Address", FieldName: "street", DataSourceHash: 22, Path: "query.users.edges.node.address.street", ParentPath: "query.users.edges.node.address", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "User", FieldName: "firstName", DataSourceHash: 11, Path: "query.users.edges.node.firstName", ParentPath: "query.users.edges.node", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "User", FieldName: "id", DataSourceHash: 11, Path: "query.users.edges.node.id", ParentPath: "query.users.edges.node", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
						{TypeName: "User", FieldName: "lastName", DataSourceHash: 11, Path: "query.users.edges.node.lastName", ParentPath: "query.users.edges.node", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
					}),
				},
				{
					dsOrder: []int{1, 0},
					suggestions: newNodeSuggestions([]NodeSuggestion{
						{TypeName: "Query", FieldName: "users", DataSourceHash: 11, Path: "query.users", ParentPath: "query", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "PaginatedUser", FieldName: "edges", DataSourceHash: 11, Path: "query.users.edges", ParentPath: "query.users", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
						{TypeName: "UserToEdge", FieldName: "node", DataSourceHash: 11, Path: "query.users.edges.node", ParentPath: "query.users.edges", IsRootNode: false, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
						{TypeName: "User", FieldName: "address", DataSourceHash: 22, Path: "query.users.edges.node.address", ParentPath: "query.users.edges.node", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "Address", FieldName: "street", DataSourceHash: 22, Path: "query.users.edges.node.address.street", ParentPath: "query.users.edges.node.address", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "User", FieldName: "firstName", DataSourceHash: 11, Path: "query.users.edges.node.firstName", ParentPath: "query.users.edges.node", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
						{TypeName: "User", FieldName: "id", DataSourceHash: 11, Path: "query.users.edges.node.id", ParentPath: "query.users.edges.node", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage2: node on the same source as selected parent"}},
						{TypeName: "User", FieldName: "lastName", DataSourceHash: 11, Path: "query.users.edges.node.lastName", ParentPath: "query.users.edges.node", IsRootNode: true, Selected: true, SelectionReasons: []string{"stage1: unique"}},
					}),
				},
			},
		},
	}

	run := func(t *testing.T, Definition, Query string, DataSources []DataSource, expected *NodeSuggestions) {
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
		dsFilter.EnableSelectionReasons()
		dsFilter.dataSources = DataSources
		planned, _ := dsFilter.findBestDataSourceSet(nil, nil)
		if report.HasErrors() {
			t.Fatal(report.Error())
		}

		// zero field refs
		for i := range planned.items {
			planned.items[i].FieldRef = 0
		}

		// remove not selected items
		actualItems := slices.DeleteFunc(planned.items, func(n *NodeSuggestion) bool {
			return n.Selected == false
		})

		// set default values for the fields that are not relevant for the test
		for item := range slices.Values(actualItems) {
			item.treeNodeId = 0
			item.IsLeaf = false
			if !item.onFragment {
				item.parentPathWithoutFragment = ""
			}
		}

		// sort suggestions by path, to have a predictable order
		// which could depend on the order of datasource as we mutate their order
		slices.SortFunc(actualItems, func(a, b *NodeSuggestion) int {
			return strings.Compare(a.Path, b.Path)
		})

		if !assert.Equal(t, expected.items, actualItems) {
			if diff := pretty.Compare(expected.items, actualItems); diff != "" {
				t.Errorf("Result don't match(-want +got)\n%s", diff)
			}
		}
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.Description, func(t *testing.T) {

			if tc.ExpectedSuggestions != nil {
				dataSourcePermutations := permutations.Generate(tc.DataSources)

				for i := range dataSourcePermutations {
					permutation := dataSourcePermutations[i]
					t.Run(fmt.Sprintf("permutation %v", permutation.Order), func(t *testing.T) {
						run(t, tc.Definition, tc.Query, permutation.DataSources, tc.ExpectedSuggestions)
					})
				}

				return
			}

			for i, variant := range tc.ExpectedVariants {
				variant := variant
				t.Run(fmt.Sprintf("Variant: %d", i), func(t *testing.T) {
					run(t, tc.Definition, tc.Query, permutations.OrderDS(tc.DataSources, variant.dsOrder), variant.suggestions)
				})
			}
		})
	}
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
