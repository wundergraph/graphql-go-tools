package plan

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type dsBuilder struct {
	ds *DataSourceConfiguration
}

func dsb() *dsBuilder { return &dsBuilder{ds: &DataSourceConfiguration{}} }

func (b *dsBuilder) RootNodeFields(typeName string, fieldNames ...string) *dsBuilder {
	b.ds.RootNodes = append(b.ds.RootNodes, TypeField{TypeName: typeName, FieldNames: fieldNames})
	return b
}

func (b *dsBuilder) RootNode(typeName string, fieldName string) *dsBuilder {
	return b.RootNodeFields(typeName, fieldName)
}

func (b *dsBuilder) ChildNodeFields(typeName string, fieldNames ...string) *dsBuilder {
	b.ds.ChildNodes = append(b.ds.ChildNodes, TypeField{TypeName: typeName, FieldNames: fieldNames})
	return b
}

func (b *dsBuilder) ChildNode(typeName string, fieldName string) *dsBuilder {
	return b.ChildNodeFields(typeName, fieldName)
}

func (b *dsBuilder) Schema(schema string) *dsBuilder {
	b.ds.Custom = []byte(schema)
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

func TestFindBestDataSourceSet(t *testing.T) {
	testCases := []struct {
		Definition  string
		Query       string
		DataSources []DataSourceConfiguration
		Expected    NodeSuggestions
	}{
		// Remove the 3rd data source, we don't use it
		{
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
				`).RootNodeFields("User", "id", "name", "surname").DS(),
				dsb().Hash(33).Schema(`
					type User @key(fields: "id") {
						id: Int
						age: Int
					}
				`).RootNodeFields("User", "id", "age").DS(),
			},
			Expected: NodeSuggestions{
				{TypeName: "Query", FieldName: "user", DataSourceHash: 11, Path: "query.user", ParentPath: "query", IsRootNode: true, preserve: true},
				{TypeName: "User", FieldName: "id", DataSourceHash: 11, Path: "query.user.id", ParentPath: "query.user", IsRootNode: true, preserve: true},
				{TypeName: "User", FieldName: "name", DataSourceHash: 22, Path: "query.user.name", ParentPath: "query.user", IsRootNode: true, preserve: true},
				{TypeName: "User", FieldName: "surname", DataSourceHash: 22, Path: "query.user.surname", ParentPath: "query.user", IsRootNode: true, preserve: true},
			},
		},
		// Pick the first and the third data sources, ignore the ones that result in more queries
		{
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
				`).RootNode("Query", "user").RootNodeFields("User", "id", "age").DS(),
				dsb().Hash(22).Schema(`
					type User @key(fields: "id") {
						id: Int
						age: Int
						name: String
					}
				`).RootNodeFields("User", "id", "age", "name").DS(),
				dsb().Hash(33).Schema(`
					type User @key(fields: "id") {
						id: Int
						name: String
						surname: String
					}
				`).RootNodeFields("User", "id", "name", "surname").DS(),
				dsb().Hash(44).Schema(`
					type User @key(fields: "id") {
						id: Int
						age: Int
					}
				`).RootNodeFields("User", "id", "age").DS(),
				dsb().Hash(55).Schema(`
					type User @key(fields: "id") {
						id: Int
						name: String
					}
				`).RootNodeFields("User", "id", "name").DS(),
			},
			Expected: NodeSuggestions{
				{TypeName: "Query", FieldName: "user", DataSourceHash: 11, Path: "query.user", ParentPath: "query", IsRootNode: true, preserve: true},
				{TypeName: "User", FieldName: "age", DataSourceHash: 11, Path: "query.user.age", ParentPath: "query.user", IsRootNode: true, preserve: true},
				{TypeName: "User", FieldName: "name", DataSourceHash: 33, Path: "query.user.name", ParentPath: "query.user", IsRootNode: true, preserve: true},
				{TypeName: "User", FieldName: "surname", DataSourceHash: 33, Path: "query.user.surname", ParentPath: "query.user", IsRootNode: true, preserve: true},
			},
		},
		// Entities: User, Address, Lines
		{
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
					RootNodeFields("User", "id", "name").DS(),
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
					RootNodeFields("User", "id", "details", "name").
					ChildNodeFields("Details", "address", "age").
					RootNodeFields("Address", "id", "name", "lines").
					RootNodeFields("Lines", "id").
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
					RootNodeFields("Address", "id", "lines").
					RootNodeFields("Lines", "id", "line1").
					DS(),
				dsb().Hash(44).Schema(`
					# sub4
					type Lines @key(fields: "id") {
						id: Int
						line1: String
						line2: String
					}
				`).
					RootNodeFields("Lines", "id", "line1", "line2").
					DS(),
			},
			Expected: NodeSuggestions{
				{TypeName: "Query", FieldName: "user", DataSourceHash: 22, Path: "query.user", ParentPath: "query", IsRootNode: true, preserve: true},
				{TypeName: "User", FieldName: "name", DataSourceHash: 22, Path: "query.user.name", ParentPath: "query.user", IsRootNode: true, preserve: true},
				{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.user.details", ParentPath: "query.user", IsRootNode: true, preserve: true},
				{TypeName: "Details", FieldName: "age", DataSourceHash: 22, Path: "query.user.details.age", ParentPath: "query.user.details", IsRootNode: false, preserve: true},
				{TypeName: "Details", FieldName: "address", DataSourceHash: 22, Path: "query.user.details.address", ParentPath: "query.user.details", IsRootNode: false, preserve: true},
				{TypeName: "Address", FieldName: "name", DataSourceHash: 22, Path: "query.user.details.address.name", ParentPath: "query.user.details.address", IsRootNode: true, preserve: true},
				{TypeName: "Address", FieldName: "lines", DataSourceHash: 22, Path: "query.user.details.address.lines", ParentPath: "query.user.details.address", IsRootNode: true, preserve: true},
				{TypeName: "Lines", FieldName: "id", DataSourceHash: 22, Path: "query.user.details.address.lines.id", ParentPath: "query.user.details.address.lines", IsRootNode: true, preserve: true},
				{TypeName: "Lines", FieldName: "line1", DataSourceHash: 44, Path: "query.user.details.address.lines.line1", ParentPath: "query.user.details.address.lines", IsRootNode: true, preserve: true},
				{TypeName: "Lines", FieldName: "line2", DataSourceHash: 44, Path: "query.user.details.address.lines.line2", ParentPath: "query.user.details.address.lines", IsRootNode: true, preserve: true},
			},
		},
		{
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
					RootNodeFields("User", "id", "details").
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
					RootNodeFields("User", "id", "details").
					ChildNode("Details", "surname").
					DS(),
			},
			Expected: NodeSuggestions{
				{TypeName: "Query", FieldName: "me", DataSourceHash: 11, Path: "query.me", ParentPath: "query", IsRootNode: true, preserve: true},
				{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, preserve: true},
				{TypeName: "Details", FieldName: "surname", DataSourceHash: 22, Path: "query.me.details.surname", ParentPath: "query.me.details", IsRootNode: false, preserve: true},
			},
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.Query, func(t *testing.T) {
			definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(tc.Definition)
			operation := unsafeparser.ParseGraphqlDocumentString(tc.Query)

			report := operationreport.Report{}
			planned := findBestDataSourceSet(&operation, &definition, &report, shuffleDS(tc.DataSources))
			if report.HasErrors() {
				t.Fatal(report.Error())
			}

			assert.ElementsMatch(t, tc.Expected, planned)
		})
	}
}

// shuffleDS randomizes the order of the data sources
// to ensure that the order doesn't matter
func shuffleDS(dataSources []DataSourceConfiguration) []DataSourceConfiguration {
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(dataSources), func(i, j int) {
		dataSources[i], dataSources[j] = dataSources[j], dataSources[i]
	})

	return dataSources
}
