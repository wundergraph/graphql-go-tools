package main

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
)

type dsBuilder struct {
	ds DataSourceConfiguration
}

func dsb() *dsBuilder { return &dsBuilder{} }

func (b *dsBuilder) RootNodeFields(typeName string, fieldNames []string) *dsBuilder {
	b.ds.RootNodes = append(b.ds.RootNodes, TypeField{TypeName: typeName, FieldNames: fieldNames})
	return b
}

func (b *dsBuilder) RootNode(typeName string, fieldName string) *dsBuilder {
	return b.RootNodeFields(typeName, []string{fieldName})
}

func (b *dsBuilder) ChildNodeFields(typeName string, fieldNames []string) *dsBuilder {
	b.ds.ChildNodes = append(b.ds.ChildNodes, TypeField{TypeName: typeName, FieldNames: fieldNames})
	return b
}

func (b *dsBuilder) ChildNode(typeName string, fieldName string) *dsBuilder {
	return b.ChildNodeFields(typeName, []string{fieldName})
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
				dsb().RootNode("Query", "user").ChildNode("User", "id").DS(),
				dsb().RootNode("User", "name").RootNode("User", "surname").DS(),
				dsb().RootNode("User", "age").DS(),
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
				dsb().RootNode("Query", "user").ChildNode("User", "age").DS(),
				dsb().RootNode("User", "age").RootNode("User", "name").DS(),
				dsb().RootNode("User", "name").RootNode("User", "surname").DS(),
				dsb().RootNode("User", "age").DS(),
				dsb().RootNode("User", "name").DS(),
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
		// Initial example from SVG
		{
			Definition: `
						type Query {
							user: User
						}
						type User {
							name: String
							details: Details
						}
						type Details {
							age: Int
							address: Address
						}
						type Address {
							name: String
							lines: Lines
						}
						type Lines {
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
				dsb().
					RootNode("Query", "user").
					ChildNode("User", "name").DS(),
				// sub2
				dsb().
					RootNode("Query", "user").
					ChildNode("Details", "address").
					ChildNode("User", "details").
					ChildNode("User", "name").
					ChildNode("Details", "age").
					ChildNode("Details", "address").
					ChildNode("Address", "name").
					ChildNode("Address", "lines").
					DS(),
				// sub3
				dsb().
					ChildNode("Details", "address").
					ChildNode("Address", "lines").
					ChildNode("Lines", "line1").
					DS(),
				// sub4
				dsb().
					RootNode("Lines", "line1").
					RootNode("Lines", "line2").DS(),
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
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.Query, func(t *testing.T) {
			// t.Parallel()

			definition, report := astparser.ParseGraphqlDocumentString(tc.Definition)
			if report.HasErrors() {
				t.Fatal(report.Error())
			}
			if err := asttransform.MergeDefinitionWithBaseSchema(&definition); err != nil {
				t.Fatal(err)
			}
			operation, report := astparser.ParseGraphqlDocumentString(tc.Query)
			if report.HasErrors() {
				t.Fatal(report.Error())
			}
			planned, err := PlanDataSources(&operation, &definition, &report, tc.DataSources)
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
