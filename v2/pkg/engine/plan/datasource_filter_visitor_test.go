package plan

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type dsBuilder struct {
	ds DataSourceConfiguration
}

func dsb() *dsBuilder { return &dsBuilder{} }

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

func (b *dsBuilder) DS() DataSourceConfiguration {
	if len(b.ds.Custom) == 0 {
		panic("schema not set")
	}
	b.ds.Hash()
	return b.ds
}

type expectedDataSource struct {
	Index     int
	UsedNodes []*UsedNode
}

func TestFindBestDataSourceSet(t *testing.T) {
	testCases := []struct {
		Definition  string
		Query       string
		DataSources []DataSourceConfiguration
		Expected    []expectedDataSource
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
				dsb().Schema(`
					type Query {
						user: User
					}
					type User @key(fields: "id") {
						id: Int
					}
				`).RootNode("Query", "user").RootNode("User", "id").DS(),
				dsb().Schema(`
					type User @key(fields: "id") {
						id: Int
						name: String
						surname: String
					}
				`).RootNodeFields("User", "id", "name", "surname").DS(),
				dsb().Schema(`
					type User @key(fields: "id") {
						id: Int
						age: Int
					}
				`).RootNodeFields("User", "id", "age").DS(),
			},
			Expected: []expectedDataSource{
				{
					Index:     0,
					UsedNodes: []*UsedNode{{"Query", "user"}, {"User", "id"}},
				},
				{
					Index:     1,
					UsedNodes: []*UsedNode{{"User", "name"}, {"User", "surname"}},
				},
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
				dsb().Schema(`
					type Query {
						user: User
					}
					type User @key(fields: "id") {
						id: Int
						age: Int
					}
				`).RootNode("Query", "user").RootNodeFields("User", "id", "age").DS(),
				dsb().Schema(`
					type User @key(fields: "id") {
						id: Int
						age: Int
						name: String
					}
				`).RootNodeFields("User", "id", "age", "name").DS(),
				dsb().Schema(`
					type User @key(fields: "id") {
						id: Int
						name: String
						surname: String
					}
				`).RootNodeFields("User", "id", "name", "surname").DS(),
				dsb().Schema(`
					type User @key(fields: "id") {
						id: Int
						age: Int
					}
				`).RootNodeFields("User", "id", "age").DS(),
				dsb().Schema(`
					type User @key(fields: "id") {
						id: Int
						name: String
					}
				`).RootNodeFields("User", "id", "name").DS(),
			},
			Expected: []expectedDataSource{
				{
					Index:     0,
					UsedNodes: []*UsedNode{{"Query", "user"}, {"User", "age"}},
				},
				{
					Index:     2,
					UsedNodes: []*UsedNode{{"User", "name"}, {"User", "surname"}},
				},
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
				dsb().Schema(`
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
				dsb().Schema(`
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
				dsb().Schema(`
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
				dsb().Schema(`
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
			Expected: []expectedDataSource{
				{
					Index: 1,
					UsedNodes: []*UsedNode{
						{"Query", "user"},
						{"User", "name"},
						{"User", "details"},
						{"Details", "age"},
						{"Details", "address"},
						{"Address", "name"},
						{"Address", "lines"},
						{"Lines", "id"},
					},
				},
				{
					Index: 3,
					UsedNodes: []*UsedNode{
						{"Lines", "line1"},
						{"Lines", "line2"},
					},
				},
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
				dsb().Schema(`
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
				dsb().Schema(`
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
			Expected: []expectedDataSource{
				{
					Index:     0,
					UsedNodes: []*UsedNode{{"Query", "me"}},
				},
				{
					Index:     1,
					UsedNodes: []*UsedNode{{"User", "details"}, {"Details", "surname"}},
				},
			},
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.Query, func(t *testing.T) {
			definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(tc.Definition)
			operation := unsafeparser.ParseGraphqlDocumentString(tc.Query)

			var expected []*UsedDataSourceConfiguration
			for _, exp := range tc.Expected {
				expected = append(expected, &UsedDataSourceConfiguration{
					DataSource: tc.DataSources[exp.Index],
					UsedNodes:  exp.UsedNodes,
				})
			}

			report := operationreport.Report{}
			planned, err := findBestDataSourceSet(&operation, &definition, &report, shuffleDS(tc.DataSources))
			if err != nil {
				t.Fatal(err)
			}
			if report.HasErrors() {
				t.Fatal(report.Error())
			}

			if !assert.ElementsMatch(t, expected, planned) {
				fmt.Println("expected:")
			}
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
