package plan

import (
	"testing"

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

func (b *dsBuilder) DS() DataSourceConfiguration {
	return b.ds
}

type expectedDataSource struct {
	Index     int
	UsedNodes []*UsedNode
}

func TestVisitDataSource(t *testing.T) {
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
				/*
					type Query {
						user: User
					}
					type User @key(fields: "id") {
						id: Int
					}
				*/
				dsb().RootNode("Query", "user").RootNode("User", "id").DS(),
				/*
					type User @key(fields: "id") {
						id: Int
						name: String
						surname: String
					}
				*/
				dsb().RootNodeFields("User", "id", "name", "surname").DS(),
				/*
					type User @key(fields: "id") {
						id: Int
						age: Int
					}
				*/
				dsb().RootNodeFields("User", "id", "age").DS(),
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
				/*
					type Query {
						user: User
					}
					type User @key(fields: "id") {
						id: Int
						age: Int
					}
				*/
				dsb().RootNode("Query", "user").RootNodeFields("User", "id", "age").DS(),
				/*
					type User @key(fields: "id") {
						id: Int
						age: Int
						name: String
					}
				*/
				dsb().RootNodeFields("User", "id", "age", "name").DS(),
				/*
					type User @key(fields: "id") {
						id: Int
						name: String
						surname: String
					}
				*/
				dsb().RootNodeFields("User", "id", "name", "surname").DS(),
				/*
					type User @key(fields: "id") {
						id: Int
						age: Int
					}
				*/
				dsb().RootNodeFields("User", "id", "age").DS(),
				/*
					type User @key(fields: "id") {
						id: Int
						name: String
					}
				*/
				dsb().RootNodeFields("User", "id", "name").DS(),
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
											line1
											line2
										}
									}
								}
							}
						}
					`,
			DataSources: []DataSourceConfiguration{
				// sub1
				/*
					type Query {
						user: User
					}
					type User @key(fields: "id") {
						id: Int
						name: String
					}
				*/
				dsb().
					RootNode("Query", "user").
					RootNodeFields("User", "id", "name").DS(),
				// sub2
				/*
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
				*/
				dsb().
					RootNode("Query", "user").
					RootNodeFields("User", "id", "details", "name").
					ChildNodeFields("Details", "address", "age").
					RootNodeFields("Address", "id", "name", "lines").
					RootNodeFields("Lines", "id").
					DS(),
				// sub3
				/*
					type Address @key(fields: "id") {
						id: Int
						lines: Lines
					}
					type Lines @key(fields: "id") {
						id: Int
						line1: String
					}
				*/
				dsb().
					RootNodeFields("Address", "id", "lines").
					RootNodeFields("Lines", "id", "line1").
					DS(),
				// sub4
				/*
					type Lines @key(fields: "id") {
						id: Int
						line1: String
						line2: String
					}
				*/
				dsb().
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
				/*
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
				*/
				dsb().
					RootNode("Query", "me").
					RootNodeFields("User", "id", "details").
					ChildNode("Details", "name").
					DS(),
				/*
					type User @key(fields: "id") {
						id: Int
						details: Details
					}
					type Details {
						surname: String
					}
				*/
				dsb().
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

			report := operationreport.Report{}
			planned, err := findBestDataSourceSet(&operation, &definition, &report, tc.DataSources)
			if err != nil {
				t.Fatal(err)
			}
			if report.HasErrors() {
				t.Fatal(report.Error())
			}
			var expected []*UsedDataSourceConfiguration
			for _, exp := range tc.Expected {
				expected = append(expected, &UsedDataSourceConfiguration{
					DataSource: tc.DataSources[exp.Index],
					UsedNodes:  exp.UsedNodes,
				})
			}
			assert.Equal(t, expected, planned)
		})
	}
}
