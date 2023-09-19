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
		Description      string
		Definition       string
		Query            string
		DataSources      []DataSourceConfiguration
		ExpectedVariants []NodeSuggestions
		CustomMatch      bool
	}{
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
			ExpectedVariants: []NodeSuggestions{{
				{TypeName: "Query", FieldName: "user", DataSourceHash: 11, Path: "query.user", ParentPath: "query", IsRootNode: true, selected: true},
				{TypeName: "User", FieldName: "id", DataSourceHash: 11, Path: "query.user.id", ParentPath: "query.user", IsRootNode: true, selected: true},
				{TypeName: "User", FieldName: "name", DataSourceHash: 22, Path: "query.user.name", ParentPath: "query.user", IsRootNode: true, selected: true},
				{TypeName: "User", FieldName: "surname", DataSourceHash: 22, Path: "query.user.surname", ParentPath: "query.user", IsRootNode: true, selected: true},
			}},
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
			ExpectedVariants: []NodeSuggestions{{
				{TypeName: "Query", FieldName: "user", DataSourceHash: 11, Path: "query.user", ParentPath: "query", IsRootNode: true, selected: true},
				{TypeName: "User", FieldName: "age", DataSourceHash: 11, Path: "query.user.age", ParentPath: "query.user", IsRootNode: true, selected: true},
				{TypeName: "User", FieldName: "name", DataSourceHash: 33, Path: "query.user.name", ParentPath: "query.user", IsRootNode: true, selected: true},
				{TypeName: "User", FieldName: "surname", DataSourceHash: 33, Path: "query.user.surname", ParentPath: "query.user", IsRootNode: true, selected: true},
			}},
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
			ExpectedVariants: []NodeSuggestions{{
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
			}},
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
			ExpectedVariants: []NodeSuggestions{{
				{TypeName: "Query", FieldName: "me", DataSourceHash: 11, Path: "query.me", ParentPath: "query", IsRootNode: true, selected: true},
				{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
				{TypeName: "Details", FieldName: "surname", DataSourceHash: 22, Path: "query.me.details.surname", ParentPath: "query.me.details", IsRootNode: false, selected: true},
			}},
		},
		{
			Description: "Shareable: 2 ds are equal so choose first available",
			Definition:  shareableDefinion,
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
			ExpectedVariants: []NodeSuggestions{
				{
					{TypeName: "Query", FieldName: "me", DataSourceHash: 11, Path: "query.me", ParentPath: "query", IsRootNode: true, selected: true},
					{TypeName: "User", FieldName: "details", DataSourceHash: 11, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
					{TypeName: "Details", FieldName: "forename", DataSourceHash: 11, Path: "query.me.details.forename", ParentPath: "query.me.details", IsRootNode: false, selected: true},
				},
				{
					{TypeName: "Query", FieldName: "me", DataSourceHash: 22, Path: "query.me", ParentPath: "query", IsRootNode: true, selected: true},
					{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
					{TypeName: "Details", FieldName: "forename", DataSourceHash: 22, Path: "query.me.details.forename", ParentPath: "query.me.details", IsRootNode: false, selected: true},
				},
			},
		},
		{
			Description: "Shareable: choose second it provides more fields",
			Definition:  shareableDefinion,
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
			ExpectedVariants: []NodeSuggestions{{
				{TypeName: "Query", FieldName: "me", DataSourceHash: 22, Path: "query.me", ParentPath: "query", IsRootNode: true, selected: true},
				{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
				{TypeName: "Details", FieldName: "forename", DataSourceHash: 22, Path: "query.me.details.forename", ParentPath: "query.me.details", IsRootNode: false, selected: true},
				{TypeName: "Details", FieldName: "surname", DataSourceHash: 22, Path: "query.me.details.surname", ParentPath: "query.me.details", IsRootNode: false, selected: true},
			}},
		},
		{
			Description: "Shareable: should use 2 ds",
			Definition:  shareableDefinion,
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
			ExpectedVariants: []NodeSuggestions{
				{
					{TypeName: "Query", FieldName: "me", DataSourceHash: 22, Path: "query.me", ParentPath: "query", IsRootNode: true, selected: true},
					{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
					{TypeName: "User", FieldName: "details", DataSourceHash: 11, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
					{TypeName: "Details", FieldName: "forename", DataSourceHash: 22, Path: "query.me.details.forename", ParentPath: "query.me.details", IsRootNode: false, selected: true},
					{TypeName: "Details", FieldName: "surname", DataSourceHash: 22, Path: "query.me.details.surname", ParentPath: "query.me.details", IsRootNode: false, selected: true},
					{TypeName: "Details", FieldName: "middlename", DataSourceHash: 11, Path: "query.me.details.middlename", ParentPath: "query.me.details", IsRootNode: false, selected: true},
				},
				{
					{TypeName: "Query", FieldName: "me", DataSourceHash: 11, Path: "query.me", ParentPath: "query", IsRootNode: true, selected: true},
					{TypeName: "User", FieldName: "details", DataSourceHash: 11, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
					{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
					{TypeName: "Details", FieldName: "forename", DataSourceHash: 11, Path: "query.me.details.forename", ParentPath: "query.me.details", IsRootNode: false, selected: true},
					{TypeName: "Details", FieldName: "surname", DataSourceHash: 22, Path: "query.me.details.surname", ParentPath: "query.me.details", IsRootNode: false, selected: true},
					{TypeName: "Details", FieldName: "middlename", DataSourceHash: 11, Path: "query.me.details.middlename", ParentPath: "query.me.details", IsRootNode: false, selected: true},
				},
			},
		},
		{
			Description: "Shareable: should use 2 ds for single field",
			Definition:  shareableDefinion,
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
			ExpectedVariants: []NodeSuggestions{
				{
					{TypeName: "Query", FieldName: "me", DataSourceHash: 22, Path: "query.me", ParentPath: "query", IsRootNode: true, selected: true},
					{TypeName: "User", FieldName: "details", DataSourceHash: 33, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
					{TypeName: "Details", FieldName: "age", DataSourceHash: 33, Path: "query.me.details.age", ParentPath: "query.me.details", IsRootNode: false, selected: true},
				},
				{
					{TypeName: "Query", FieldName: "me", DataSourceHash: 11, Path: "query.me", ParentPath: "query", IsRootNode: true, selected: true},
					{TypeName: "User", FieldName: "details", DataSourceHash: 33, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
					{TypeName: "Details", FieldName: "age", DataSourceHash: 33, Path: "query.me.details.age", ParentPath: "query.me.details", IsRootNode: false, selected: true},
				},
			},
		},
		{
			Description: "Shareable: should use all ds",
			Definition:  shareableDefinion,
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
			CustomMatch: true,
			ExpectedVariants: []NodeSuggestions{
				{
					{TypeName: "Query", FieldName: "me", DataSourceHash: 11, Path: "query.me", ParentPath: "query", IsRootNode: true, selected: true},
					{TypeName: "User", FieldName: "details", DataSourceHash: 11, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
					{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
					{TypeName: "User", FieldName: "details", DataSourceHash: 33, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
					{TypeName: "Details", FieldName: "forename", DataSourceHash: 11, Path: "query.me.details.forename", ParentPath: "query.me.details", IsRootNode: false, selected: true},
					{TypeName: "Details", FieldName: "surname", DataSourceHash: 22, Path: "query.me.details.surname", ParentPath: "query.me.details", IsRootNode: false, selected: true},
					{TypeName: "Details", FieldName: "middlename", DataSourceHash: 11, Path: "query.me.details.middlename", ParentPath: "query.me.details", IsRootNode: false, selected: true},
					{TypeName: "Details", FieldName: "age", DataSourceHash: 33, Path: "query.me.details.age", ParentPath: "query.me.details", IsRootNode: false, selected: true},
				},
				{
					{TypeName: "Query", FieldName: "me", DataSourceHash: 22, Path: "query.me", ParentPath: "query", IsRootNode: true, selected: true},
					{TypeName: "User", FieldName: "details", DataSourceHash: 11, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
					{TypeName: "User", FieldName: "details", DataSourceHash: 22, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
					{TypeName: "User", FieldName: "details", DataSourceHash: 33, Path: "query.me.details", ParentPath: "query.me", IsRootNode: true, selected: true},
					{TypeName: "Details", FieldName: "forename", DataSourceHash: 22, Path: "query.me.details.forename", ParentPath: "query.me.details", IsRootNode: false, selected: true},
					{TypeName: "Details", FieldName: "surname", DataSourceHash: 22, Path: "query.me.details.surname", ParentPath: "query.me.details", IsRootNode: false, selected: true},
					{TypeName: "Details", FieldName: "middlename", DataSourceHash: 11, Path: "query.me.details.middlename", ParentPath: "query.me.details", IsRootNode: false, selected: true},
					{TypeName: "Details", FieldName: "age", DataSourceHash: 33, Path: "query.me.details.age", ParentPath: "query.me.details", IsRootNode: false, selected: true},
				},
			},
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.Description, func(t *testing.T) {
			definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(tc.Definition)
			operation := unsafeparser.ParseGraphqlDocumentString(tc.Query)
			report := operationreport.Report{}

			dsFilter := NewDataSourceFilter(&operation, &definition, &report)

			planned := dsFilter.findBestDataSourceSet(shuffleDS(tc.DataSources), nil)
			if report.HasErrors() {
				t.Fatal(report.Error())
			}

			if len(tc.ExpectedVariants) == 1 {
				assert.Equal(t, tc.ExpectedVariants[0], planned)
			} else if tc.CustomMatch {
				// custom match is required because order of variants is not guaranteed
				errs := &customT{}
				for _, variant := range tc.ExpectedVariants {
					if assert.ElementsMatch(errs, variant, planned) {
						break
					}
				}
				if len(errs.errMsgs) == len(tc.ExpectedVariants) {
					t.Errorf("errors:\n%s", errs.errMsgs)
				}
			} else {
				assert.Contains(t, tc.ExpectedVariants, planned)
			}
		})
	}
}

type customT struct {
	errMsgs []string
}

func (c *customT) Errorf(format string, args ...interface{}) {
	c.errMsgs = append(c.errMsgs, fmt.Sprintf(format, args...))
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

const shareableDefinion = `
	type User {
		id: ID!
		details: Details!
	}

	type Details {
		forename: String!
		surname: String!
		middlename: String!
		age: Int!
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
	}
`

var shareableDS3 = dsb().Hash(33).Schema(shareableDS3Schema).
	RootNode("User", "id", "details").
	ChildNode("Details", "age").
	DS()
