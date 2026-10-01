package operation_complexity

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestCalculateOperationComplexity(t *testing.T) {
	t.Run("query with a scalar return type", func(t *testing.T) {
		run(t, testDefinition, `
				{
				  currentPeriod
				}`,
			OperationStats{
				FieldCount: 1,
				NodeCount:  0,
				Complexity: 0,
				Depth:      0,
			},
			[]RootFieldStats{
				{
					TypeName:  "Query",
					FieldName: "currentPeriod",
					Stats: OperationStats{
						FieldCount: 1,
						NodeCount:  0,
						Complexity: 0,
						Depth:      0,
					},
				},
			},
		)
	})
	t.Run("one user", func(t *testing.T) {
		run(t, testDefinition, `
				{
				  users(first: 1) {
					__typename
					id
					balance
					name
					address {
					  city
					  country
					}
				  }
				}`,
			OperationStats{
				FieldCount: 8,
				NodeCount:  2,
				Complexity: 2,
				Depth:      3,
			},
			[]RootFieldStats{
				{
					TypeName:  "Query",
					FieldName: "users",
					Stats: OperationStats{
						FieldCount: 8,
						NodeCount:  2,
						Complexity: 2,
						Depth:      2,
					},
				},
			},
		)
	})
	t.Run("skipped node", func(t *testing.T) {
		run(t, testDefinition, `
				{
				  activeUsers {
				    id
				    balance
				    name
				  }
				}`,
			OperationStats{
				FieldCount: 0,
				NodeCount:  0,
				Complexity: 0,
				Depth:      0,
			},
			[]RootFieldStats{},
		)
	})
	t.Run("one user with inline fragments", func(t *testing.T) {
		run(t, testDefinition, `
				{
				  users(first: 1) {
					... {
						id
						balance
						name
						address {
						  city
						  country
						}
					}
				  }
				}`,
			OperationStats{
				FieldCount: 7,
				NodeCount:  2,
				Complexity: 2,
				Depth:      3,
			},
			[]RootFieldStats{
				{
					TypeName:  "Query",
					FieldName: "users",
					Stats: OperationStats{
						FieldCount: 7,
						NodeCount:  2,
						Complexity: 2,
						Depth:      2,
					},
				},
			},
		)
	})
	t.Run("one user with fragments", func(t *testing.T) {
		run(t, testDefinition, `
				{
				  users(first: 1) {
					...UserFragment
				  }
				}
				fragment UserFragment on User {
					id
					balance
					name
					address {
					  city
					  country
					}
                }
				`,
			OperationStats{
				FieldCount: 7,
				NodeCount:  2,
				Complexity: 2,
				Depth:      3,
			},
			[]RootFieldStats{
				{
					TypeName:  "Query",
					FieldName: "users",
					Stats: OperationStats{
						FieldCount: 7,
						NodeCount:  2,
						Complexity: 2,
						Depth:      2,
					},
				},
			},
		)
	})
	t.Run("multiple users", func(t *testing.T) {
		run(t, testDefinition, `
				{
				  users(first: 10) {
					id
					balance
					name
					address {
					  city
					  country
					}
				  }
				}`,
			OperationStats{
				FieldCount: 7,
				NodeCount:  20,
				Complexity: 11,
				Depth:      3,
			},
			[]RootFieldStats{
				{
					TypeName:  "Query",
					FieldName: "users",
					Stats: OperationStats{
						FieldCount: 7,
						NodeCount:  20,
						Complexity: 11,
						Depth:      2,
					},
				},
			},
		)
	})
	t.Run("multiple users with multiple transactions", func(t *testing.T) {
		run(t, testDefinition, `
				{
				  users(first: 10) {
					id
					balance
					name
					address {
					  city
					  country
					}
					transactions(first: 5) {
						id
						amount
					}
				  }
				}`,
			OperationStats{
				FieldCount: 10,
				NodeCount:  70,
				Complexity: 21,
				Depth:      3,
			},
			[]RootFieldStats{
				{
					TypeName:  "Query",
					FieldName: "users",
					Stats: OperationStats{
						FieldCount: 10,
						NodeCount:  70,
						Complexity: 21,
						Depth:      2,
					},
				},
			},
		)
	})
	t.Run("multiple users with multiple transactions with nested senders", func(t *testing.T) {
		run(t, testDefinition, `
				{
				  users(first: 10) {
					id
					balance
					name
					address {
					  city
					  country
					}
					transactions(first: 5) {
						id
						amount
						sender {
							id
							transactions(first: 10) {
								id
								amount
							}
						}
						recipient {
							id
							transactions(first: 5) {
								id
								amount
							}
						}
					}
				  }
				}`,
			OperationStats{
				FieldCount: 20,
				NodeCount:  920,
				Complexity: 221,
				Depth:      5,
			},
			[]RootFieldStats{
				{
					TypeName:  "Query",
					FieldName: "users",
					Stats: OperationStats{
						FieldCount: 20,
						NodeCount:  920,
						Complexity: 221,
						Depth:      4,
					},
				},
			},
		)
	})
	t.Run("multiple queries and one being an alias", func(t *testing.T) {
		run(t, testDefinition, `
				{
				  person: user(id: "1") {
					name
 				  }
				  users(first: 1) {
					id
					balance
					name
					address {
					  city
					  country
					}
				  }
				  bestUsers: users(first: 10) {
					id
					balance
					name
					address {
					  city
					  country
					}
					transactions(first: 5) {
						id
						amount
					}
				  }
				}`,
			OperationStats{
				FieldCount: 19,
				NodeCount:  73,
				Complexity: 24,
				Depth:      3,
			},
			[]RootFieldStats{
				{
					TypeName:  "Query",
					FieldName: "user",
					Alias:     "person",
					Stats: OperationStats{
						FieldCount: 2,
						NodeCount:  1,
						Complexity: 1,
						Depth:      1,
					},
				},
				{
					TypeName:  "Query",
					FieldName: "users",
					Stats: OperationStats{
						FieldCount: 7,
						NodeCount:  2,
						Complexity: 2,
						Depth:      2,
					},
				},
				{
					TypeName:  "Query",
					FieldName: "users",
					Alias:     "bestUsers",
					Stats: OperationStats{
						FieldCount: 10,
						NodeCount:  70,
						Complexity: 21,
						Depth:      2,
					},
				},
			},
		)
	})
	t.Run("multiple queries with different depth higher depth first", func(t *testing.T) {
		run(t, testDefinition, `
				{
					transactions(first: 1) {
						id
						sender {
							id
							transactions(first: 1) {
								id
							}
						}
					}
					users(first: 1) {
						id
						address {
					  		city
						}
					}
				}`,
			OperationStats{
				FieldCount: 10,
				NodeCount:  5,
				Complexity: 5,
				Depth:      4,
			},
			[]RootFieldStats{
				{
					TypeName:  "Query",
					FieldName: "transactions",
					Stats: OperationStats{
						FieldCount: 6,
						NodeCount:  3,
						Complexity: 3,
						Depth:      3,
					},
				},
				{
					TypeName:  "Query",
					FieldName: "users",
					Stats: OperationStats{
						FieldCount: 4,
						NodeCount:  2,
						Complexity: 2,
						Depth:      2,
					},
				},
			},
		)
	})
	t.Run("multiple queries with different depth higher depth last", func(t *testing.T) {
		run(t, testDefinition, `
				{
					users(first: 1) {
						id
						address {
					  		city
						}
					}
					transactions(first: 1) {
						id
						sender {
							id
							transactions(first: 1) {
								id
							}
						}
					}
				}`,
			OperationStats{
				FieldCount: 10,
				NodeCount:  5,
				Complexity: 5,
				Depth:      4,
			},
			[]RootFieldStats{
				{
					TypeName:  "Query",
					FieldName: "users",
					Stats: OperationStats{
						FieldCount: 4,
						NodeCount:  2,
						Complexity: 2,
						Depth:      2,
					},
				},
				{
					TypeName:  "Query",
					FieldName: "transactions",
					Stats: OperationStats{
						FieldCount: 6,
						NodeCount:  3,
						Complexity: 3,
						Depth:      3,
					},
				},
			},
		)
	})
	t.Run("multiple mutations with alias", func(t *testing.T) {
		run(t, testDefinition, `
				mutation AlterUsers {
				  createJohn: createUser(input: {balance: 10, name: "John Doe", email: "john@doe.fake"}) {
                    id
				  }
				  createJane: createUser(input: {balance: 100, name: "Jane Doe", email: "jane@doe.fake"}) {
                    id
				  }
				}`,
			OperationStats{
				FieldCount: 4,
				NodeCount:  2,
				Complexity: 2,
				Depth:      2,
			},
			[]RootFieldStats{
				{
					TypeName:  "Mutation",
					FieldName: "createUser",
					Alias:     "createJohn",
					Stats: OperationStats{
						FieldCount: 2,
						NodeCount:  1,
						Complexity: 1,
						Depth:      1,
					},
				},
				{
					TypeName:  "Mutation",
					FieldName: "createUser",
					Alias:     "createJane",
					Stats: OperationStats{
						FieldCount: 2,
						NodeCount:  1,
						Complexity: 1,
						Depth:      1,
					},
				},
			},
		)
	})
	t.Run("introspection query", func(t *testing.T) {
		run(t, testDefinition, introspectionQuery,
			OperationStats{
				FieldCount: 181,
				NodeCount:  59,
				Complexity: 59,
				Depth:      13,
			},
			[]RootFieldStats{
				{
					TypeName:  "Query",
					FieldName: "__schema",
					Stats: OperationStats{
						FieldCount: 181,
						NodeCount:  59,
						Complexity: 59,
						Depth:      12,
					},
				},
			},
		)
	})
	t.Run("introspection query with skip", func(t *testing.T) {
		runSkipIntrospection(t, testDefinition, introspectionQuery,
			OperationStats{
				FieldCount: 0,
				NodeCount:  0,
				Complexity: 0,
				Depth:      0,
			},
			[]RootFieldStats{},
		)
	})
}

func TestCalculateOperationComplexityDepth(t *testing.T) {
	t.Run("deep path", func(t *testing.T) {
		runDepthTest(t, `{
			root {
				... on Concrete {
					next {
						... on Concrete {
							next { leaf }
						}
					}
				}
			}
		}`, OperationStats{FieldCount: 4, NodeCount: 3, Complexity: 3, Depth: 4})
	})

	t.Run("two shallower siblings", func(t *testing.T) {
		runDepthTest(t, `{
			root {
				... on Concrete {
					branchA { next { leaf } }
					branchB { leaf }
					next { next { next { leaf } } }
				}
			}
		}`, OperationStats{FieldCount: 10, NodeCount: 7, Complexity: 7, Depth: 5})
	})

	t.Run("one shallower sibling", func(t *testing.T) {
		runDepthTest(t, `{
			root {
				... on Concrete {
					branchB { leaf }
					next { next { next { leaf } } }
				}
			}
		}`, OperationStats{FieldCount: 7, NodeCount: 5, Complexity: 5, Depth: 5})
	})

	t.Run("complex input literal on root field", func(t *testing.T) {
		runDepthTest(t,
			`{ root(input: {key: "value"}) { next { leaf } } }`,
			OperationStats{FieldCount: 3, NodeCount: 2, Complexity: 2, Depth: 3},
		)
	})

	t.Run("complex input literal on nested field", func(t *testing.T) {
		runDepthTest(t,
			`{ root { next(input: {key: "value"}) { next { leaf } } } }`,
			OperationStats{FieldCount: 4, NodeCount: 3, Complexity: 3, Depth: 4},
		)
	})

	t.Run("complex input literal on leaf field", func(t *testing.T) {
		runDepthTest(t,
			`{ root { next { leaf(input: {key: "value"}) } } }`,
			OperationStats{FieldCount: 3, NodeCount: 2, Complexity: 2, Depth: 3},
		)
	})

	t.Run("complex input literal on field inside inline fragment", func(t *testing.T) {
		runDepthTest(t, `{
			root {
				... on Concrete {
					next(input: {key: "value"}) { next { leaf } }
				}
			}
		}`, OperationStats{FieldCount: 4, NodeCount: 3, Complexity: 3, Depth: 4})
	})
}

func TestCalculateOperationComplexityDepthWithRootMultiplier(t *testing.T) {
	t.Parallel()

	runDepthTest(t, `{
		root(first: 2) {
			... on Concrete {
				next { leaf }
			}
		}
	}`, OperationStats{FieldCount: 3, NodeCount: 4, Complexity: 3, Depth: 3})
}

func TestCalculateOperationComplexityDepthWithStackedMultipliers(t *testing.T) {
	t.Parallel()

	runDepthTest(t, `{
		root(first: 2) {
			... on Concrete {
				next(input: {key: "value"}, first: 3) { next { leaf } }
			}
		}
	}`, OperationStats{FieldCount: 4, NodeCount: 14, Complexity: 9, Depth: 4})
}

func runDepthTest(t *testing.T, operationString string, expectedStats OperationStats) {
	t.Helper()

	definition := unsafeparser.ParseGraphqlDocumentString(depthRegressionDefinition)
	operation := unsafeparser.ParseGraphqlDocumentString(operationString)
	report := operationreport.Report{}

	astnormalization.NormalizeOperation(&operation, &definition, &report)
	stats, rootFieldStats := NewOperationComplexityEstimator(false).Do(&operation, &definition, &report)

	require.False(t, report.HasErrors(), report.Error())
	assert.Equal(t, expectedStats, stats)
	require.Len(t, rootFieldStats, 1)
	expectedStats.Depth--
	assert.Equal(t, expectedStats, rootFieldStats[0].Stats)
}

func TestOperationComplexityEstimatorReuseAfterAbortedWalk(t *testing.T) {
	t.Parallel()

	definition := unsafeparser.ParseGraphqlDocumentString(testDefinition)
	estimator := NewOperationComplexityEstimator(false)

	invalidOperation := unsafeparser.ParseGraphqlDocumentString(`
		{
			users(first: 1) {
				transactions(first: 1) {
					sender {
						address {
							... on UnknownType {
								city
							}
						}
					}
				}
			}
		}`)
	invalidReport := operationreport.Report{}
	estimator.Do(&invalidOperation, &definition, &invalidReport)
	require.True(t, invalidReport.HasErrors())

	operation := unsafeparser.ParseGraphqlDocumentString(`
		{
			users(first: 1) {
				id
				address {
					city
				}
			}
		}`)
	wantReport := operationreport.Report{}
	wantGlobal, wantRootFields := NewOperationComplexityEstimator(false).Do(&operation, &definition, &wantReport)
	require.False(t, wantReport.HasErrors())

	gotReport := operationreport.Report{}
	gotGlobal, gotRootFields := estimator.Do(&operation, &definition, &gotReport)
	require.False(t, gotReport.HasErrors())

	assert.Equal(t, wantGlobal, gotGlobal)
	assert.Equal(t, wantRootFields, gotRootFields)
}

func TestCalculateOperationComplexityFragmentSpreads(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		operation string
		// inlined is the operation with each fragment spread replaced by an
		// inline fragment with the fragment's type condition and selections.
		inlined    string
		stats      OperationStats
		rootFields []RootFieldStats
	}{
		{
			name:      "spread in a field",
			operation: `{ me { ...UserFields } } fragment UserFields on User { id name address { city } }`,
			inlined:   `{ me { ... on User { id name address { city } } } }`,
			stats:     OperationStats{FieldCount: 5, NodeCount: 2, Complexity: 2, Depth: 3},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "me", Stats: OperationStats{FieldCount: 5, NodeCount: 2, Complexity: 2, Depth: 2}},
			},
		},
		{
			name: "spreads inside a fragment",
			operation: `
				{ me { ...UserFields } }
				fragment UserFields on User { id ...NameFields address { ...AddressFields } }
				fragment NameFields on User { name }
				fragment AddressFields on Address { city country }`,
			inlined: `{ me { ... on User { id ... on User { name } address { ... on Address { city country } } } } }`,
			stats:   OperationStats{FieldCount: 6, NodeCount: 2, Complexity: 2, Depth: 3},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "me", Stats: OperationStats{FieldCount: 6, NodeCount: 2, Complexity: 2, Depth: 2}},
			},
		},
		{
			name:      "spread inside an inline fragment",
			operation: `{ node(id: "1") { ... on User { ...UserFields } } } fragment UserFields on User { id name }`,
			inlined:   `{ node(id: "1") { ... on User { ... on User { id name } } } }`,
			stats:     OperationStats{FieldCount: 3, NodeCount: 1, Complexity: 1, Depth: 2},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "node", Stats: OperationStats{FieldCount: 3, NodeCount: 1, Complexity: 1, Depth: 1}},
			},
		},
		{
			name:      "fragment defined before the operation",
			operation: `fragment UserFields on User { id name } query { me { ...UserFields } }`,
			inlined:   `query { me { ... on User { id name } } }`,
			stats:     OperationStats{FieldCount: 3, NodeCount: 1, Complexity: 1, Depth: 2},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "me", Stats: OperationStats{FieldCount: 3, NodeCount: 1, Complexity: 1, Depth: 1}},
			},
		},
		{
			name:      "root spread on query",
			operation: `query { ...RootFields currentPeriod } fragment RootFields on Query { me { id } users(first: 2) { name } __typename }`,
			inlined:   `query { ... on Query { me { id } users(first: 2) { name } __typename } currentPeriod }`,
			stats:     OperationStats{FieldCount: 6, NodeCount: 3, Complexity: 2, Depth: 2},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "me", Stats: OperationStats{FieldCount: 2, NodeCount: 1, Complexity: 1, Depth: 1}},
				{TypeName: "Query", FieldName: "users", Stats: OperationStats{FieldCount: 2, NodeCount: 2, Complexity: 1, Depth: 1}},
				{TypeName: "Query", FieldName: "__typename", Stats: OperationStats{FieldCount: 1}},
				{TypeName: "Query", FieldName: "currentPeriod", Stats: OperationStats{FieldCount: 1}},
			},
		},
		{
			name:      "root spread with aliases",
			operation: `{ ...AliasedFields } fragment AliasedFields on Query { first: user(id: "1") { id } second: user(id: "2") { id } period: currentPeriod }`,
			inlined:   `{ ... on Query { first: user(id: "1") { id } second: user(id: "2") { id } period: currentPeriod } }`,
			stats:     OperationStats{FieldCount: 5, NodeCount: 2, Complexity: 2, Depth: 2},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "user", Alias: "first", Stats: OperationStats{FieldCount: 2, NodeCount: 1, Complexity: 1, Depth: 1}},
				{TypeName: "Query", FieldName: "user", Alias: "second", Stats: OperationStats{FieldCount: 2, NodeCount: 1, Complexity: 1, Depth: 1}},
				{TypeName: "Query", FieldName: "currentPeriod", Alias: "period", Stats: OperationStats{FieldCount: 1}},
			},
		},
		{
			name:      "root spread on mutation",
			operation: `mutation { ...Mutations } fragment Mutations on Mutation { createUser(name: "Jane") { id } removed: deleteUser(id: "1") { id name } }`,
			inlined:   `mutation { ... on Mutation { createUser(name: "Jane") { id } removed: deleteUser(id: "1") { id name } } }`,
			stats:     OperationStats{FieldCount: 5, NodeCount: 2, Complexity: 2, Depth: 2},
			rootFields: []RootFieldStats{
				{TypeName: "Mutation", FieldName: "createUser", Stats: OperationStats{FieldCount: 2, NodeCount: 1, Complexity: 1, Depth: 1}},
				{TypeName: "Mutation", FieldName: "deleteUser", Alias: "removed", Stats: OperationStats{FieldCount: 3, NodeCount: 1, Complexity: 1, Depth: 1}},
			},
		},
		{
			name: "interface spreads",
			operation: `
				{ node(id: "1") { ...NodeFields ...DogFields } }
				fragment NodeFields on Node { id __typename }
				fragment DogFields on Dog { name owner { id } }`,
			inlined: `{ node(id: "1") { ... on Node { id __typename } ... on Dog { name owner { id } } } }`,
			stats:   OperationStats{FieldCount: 6, NodeCount: 2, Complexity: 2, Depth: 3},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "node", Stats: OperationStats{FieldCount: 6, NodeCount: 2, Complexity: 2, Depth: 2}},
			},
		},
		{
			name: "union spreads",
			operation: `
				{ search(first: 3) { ...SearchFields } }
				fragment SearchFields on SearchResult { __typename ... on User { name } ...PetFields }
				fragment PetFields on Pet { name owner { id } }`,
			inlined: `{ search(first: 3) { ... on SearchResult { __typename ... on User { name } ... on Pet { name owner { id } } } } }`,
			stats:   OperationStats{FieldCount: 6, NodeCount: 6, Complexity: 4, Depth: 3},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "search", Stats: OperationStats{FieldCount: 6, NodeCount: 6, Complexity: 4, Depth: 2}},
			},
		},
		{
			name:      "multipliers around and inside a fragment",
			operation: `{ users(first: 10) { ...UserFields } } fragment UserFields on User { id friends(first: 5) { name } }`,
			inlined:   `{ users(first: 10) { ... on User { id friends(first: 5) { name } } } }`,
			stats:     OperationStats{FieldCount: 4, NodeCount: 60, Complexity: 11, Depth: 3},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "users", Stats: OperationStats{FieldCount: 4, NodeCount: 60, Complexity: 11, Depth: 2}},
			},
		},
		{
			name: "skipped fields inside fragments",
			operation: `
				{ me { ...UserFields } ...RootFields }
				fragment UserFields on User { id secret }
				fragment RootFields on Query { hidden { id name } currentPeriod }`,
			inlined: `{ me { ... on User { id secret } } ... on Query { hidden { id name } currentPeriod } }`,
			stats:   OperationStats{FieldCount: 3, NodeCount: 1, Complexity: 1, Depth: 2},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "me", Stats: OperationStats{FieldCount: 2, NodeCount: 1, Complexity: 1, Depth: 1}},
				{TypeName: "Query", FieldName: "currentPeriod", Stats: OperationStats{FieldCount: 1}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stats, rootFields := estimateUnnormalized(t, fragmentTestDefinition, tt.operation, false)
			assert.Equal(t, tt.stats, stats)
			assert.Equal(t, tt.rootFields, rootFields)

			inlinedStats, inlinedRootFields := estimateUnnormalized(t, fragmentTestDefinition, tt.inlined, false)
			assert.Equal(t, inlinedStats, stats, "unexpected stats compared to inline fragments")
			assert.Equal(t, inlinedRootFields, rootFields, "unexpected root fields compared to inline fragments")

			normalizedStats, normalizedRootFields := estimateNormalized(t, fragmentTestDefinition, tt.operation, false)
			assert.Equal(t, normalizedStats, stats, "unexpected stats compared to the normalized operation")
			assert.Equal(t, normalizedRootFields, rootFields, "unexpected root fields compared to the normalized operation")
		})
	}
}

func TestCalculateOperationComplexityFragmentSpreadsMatchInlinedQuery(t *testing.T) {
	t.Parallel()

	stats, rootFields := estimateUnnormalized(t, testDefinition, complexQueryWithFragments, false)
	wantStats, wantRootFields := estimateUnnormalized(t, testDefinition, complexQuery, false)

	assert.Equal(t, OperationStats{FieldCount: 20, NodeCount: 920, Complexity: 221, Depth: 5}, stats)
	assert.Equal(t, wantStats, stats)
	assert.Equal(t, wantRootFields, rootFields)
}

func TestCalculateOperationComplexityIntrospectionFragments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		operation         string
		skipIntrospection bool
		stats             OperationStats
		rootFields        []RootFieldStats
	}{
		{
			name:      "introspection query",
			operation: introspectionQuery,
			stats:     OperationStats{FieldCount: 181, NodeCount: 59, Complexity: 59, Depth: 13},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "__schema", Stats: OperationStats{FieldCount: 181, NodeCount: 59, Complexity: 59, Depth: 12}},
			},
		},
		{
			name:              "introspection query with skip",
			operation:         introspectionQuery,
			skipIntrospection: true,
			rootFields:        []RootFieldStats{},
		},
		{
			name:      "introspection field in a root spread",
			operation: `{ ...RootFields } fragment RootFields on Query { __schema { queryType { name } } currentPeriod }`,
			stats:     OperationStats{FieldCount: 4, NodeCount: 2, Complexity: 2, Depth: 3},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "__schema", Stats: OperationStats{FieldCount: 3, NodeCount: 2, Complexity: 2, Depth: 2}},
				{TypeName: "Query", FieldName: "currentPeriod", Stats: OperationStats{FieldCount: 1}},
			},
		},
		{
			name:              "introspection field in a root spread with skip",
			operation:         `{ ...RootFields } fragment RootFields on Query { __schema { queryType { name } } currentPeriod }`,
			skipIntrospection: true,
			stats:             OperationStats{FieldCount: 1},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "currentPeriod", Stats: OperationStats{FieldCount: 1}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stats, rootFields := estimateUnnormalized(t, testDefinition, tt.operation, tt.skipIntrospection)
			assert.Equal(t, tt.stats, stats)
			assert.Equal(t, tt.rootFields, rootFields)

			normalizedStats, normalizedRootFields := estimateNormalized(t, testDefinition, tt.operation, tt.skipIntrospection)
			assert.Equal(t, normalizedStats, stats, "unexpected stats compared to the normalized operation")
			assert.Equal(t, normalizedRootFields, rootFields, "unexpected root fields compared to the normalized operation")
		})
	}
}

func TestCalculateOperationComplexityRepeatedFragmentSpreads(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		operation  string
		stats      OperationStats
		rootFields []RootFieldStats
	}{
		{
			name:      "same fragment twice in a field",
			operation: `{ me { ...UserFields ...UserFields } } fragment UserFields on User { id name }`,
			stats:     OperationStats{FieldCount: 3, NodeCount: 1, Complexity: 1, Depth: 2},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "me", Stats: OperationStats{FieldCount: 3, NodeCount: 1, Complexity: 1, Depth: 1}},
			},
		},
		{
			name:      "same fragment again in an inline fragment on the same type",
			operation: `{ me { ...UserFields ... on User { ...UserFields } } } fragment UserFields on User { id name }`,
			stats:     OperationStats{FieldCount: 3, NodeCount: 1, Complexity: 1, Depth: 2},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "me", Stats: OperationStats{FieldCount: 3, NodeCount: 1, Complexity: 1, Depth: 1}},
			},
		},
		{
			name:      "same fragment on different enclosing types",
			operation: `{ search(first: 1) { ... on Dog { ...PetFields } ... on Cat { ...PetFields } } } fragment PetFields on Pet { name }`,
			stats:     OperationStats{FieldCount: 3, NodeCount: 1, Complexity: 1, Depth: 2},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "search", Stats: OperationStats{FieldCount: 3, NodeCount: 1, Complexity: 1, Depth: 1}},
			},
		},
		{
			name:      "same fragment in different fields",
			operation: `{ me { ...UserFields } user(id: "1") { ...UserFields } } fragment UserFields on User { id }`,
			stats:     OperationStats{FieldCount: 4, NodeCount: 2, Complexity: 2, Depth: 2},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "me", Stats: OperationStats{FieldCount: 2, NodeCount: 1, Complexity: 1, Depth: 1}},
				{TypeName: "Query", FieldName: "user", Stats: OperationStats{FieldCount: 2, NodeCount: 1, Complexity: 1, Depth: 1}},
			},
		},
		{
			name:      "same fragment in a nested field",
			operation: `{ me { ...UserFields friends(first: 2) { ...UserFields } } } fragment UserFields on User { id }`,
			stats:     OperationStats{FieldCount: 4, NodeCount: 3, Complexity: 2, Depth: 3},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "me", Stats: OperationStats{FieldCount: 4, NodeCount: 3, Complexity: 2, Depth: 2}},
			},
		},
		{
			name: "same fragments under different aliases",
			operation: `
				{ first: me { ...FriendFields } second: me { ...FriendFields } }
				fragment FriendFields on User { friends(first: 1) { ...UserFields } }
				fragment UserFields on User { id }`,
			stats: OperationStats{FieldCount: 6, NodeCount: 4, Complexity: 4, Depth: 3},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "me", Alias: "first", Stats: OperationStats{FieldCount: 3, NodeCount: 2, Complexity: 2, Depth: 2}},
				{TypeName: "Query", FieldName: "me", Alias: "second", Stats: OperationStats{FieldCount: 3, NodeCount: 2, Complexity: 2, Depth: 2}},
			},
		},
		{
			name:      "same root fragment repeatedly",
			operation: `{ ...RootFields ...RootFields currentPeriod ... on Query { ...RootFields } } fragment RootFields on Query { me { id } }`,
			stats:     OperationStats{FieldCount: 3, NodeCount: 1, Complexity: 1, Depth: 2},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "me", Stats: OperationStats{FieldCount: 2, NodeCount: 1, Complexity: 1, Depth: 1}},
				{TypeName: "Query", FieldName: "currentPeriod", Stats: OperationStats{FieldCount: 1}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stats, rootFields := estimateUnnormalized(t, fragmentTestDefinition, tt.operation, false)
			assert.Equal(t, tt.stats, stats)
			assert.Equal(t, tt.rootFields, rootFields)

			normalizedStats, normalizedRootFields := estimateNormalized(t, fragmentTestDefinition, tt.operation, false)
			assert.Equal(t, normalizedStats, stats, "unexpected stats compared to the normalized operation")
			assert.Equal(t, normalizedRootFields, rootFields, "unexpected root fields compared to the normalized operation")
		})
	}
}

func TestCalculateOperationComplexityFragmentSpreadBomb(t *testing.T) {
	t.Parallel()

	// Each fragment spreads the next one twice, so expanding every spread
	// would take 2^30 expansions.
	const levels = 30

	tests := []struct {
		name       string
		operation  string
		stats      OperationStats
		rootFields []RootFieldStats
	}{
		{
			name:      "root",
			operation: `{ ...Bomb0 }` + fragmentSpreadBomb(levels, "Query", "currentPeriod"),
			stats:     OperationStats{FieldCount: 1},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "currentPeriod", Stats: OperationStats{FieldCount: 1}},
			},
		},
		{
			name:      "field",
			operation: `{ me { ...Bomb0 } }` + fragmentSpreadBomb(levels, "User", "id"),
			stats:     OperationStats{FieldCount: 2, NodeCount: 1, Complexity: 1, Depth: 2},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "me", Stats: OperationStats{FieldCount: 2, NodeCount: 1, Complexity: 1, Depth: 1}},
			},
		},
		{
			// Each fragment spreads the next one on User and on Dog, so every
			// fragment is expanded once per type.
			name:      "type conditions",
			operation: `{ node(id: "1") { ...Bomb0 } }` + typeConditionFragmentSpreadBomb(levels),
			stats:     OperationStats{FieldCount: 3, NodeCount: 1, Complexity: 1, Depth: 2},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "node", Stats: OperationStats{FieldCount: 3, NodeCount: 1, Complexity: 1, Depth: 1}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stats, rootFields := estimateInTime(t, fragmentTestDefinition, tt.operation)
			assert.Equal(t, tt.stats, stats)
			assert.Equal(t, tt.rootFields, rootFields)
		})
	}
}

func TestCalculateOperationComplexityLargeFragmentDocuments(t *testing.T) {
	t.Parallel()

	// Both documents took far longer than the timeout when each spread scanned
	// the fragments already expanded in its scope and each fragment walk
	// visited every root node of the document.
	tests := []struct {
		name       string
		definition string
		operation  string
		stats      OperationStats
		rootFields []RootFieldStats
	}{
		{
			// 200 levels of 20 fragments on 20 types, where each fragment spreads
			// every fragment of the next level. Each fragment is expanded once per
			// type it is spread on: about 80000 expansions in one scope.
			name:       "many fragments expanded in one scope",
			definition: implementationsDefinition(20),
			operation:  `{ node { ...Level0_0 } }` + typeConditionFragmentLevels(200, 20),
			stats:      OperationStats{FieldCount: 401, NodeCount: 1, Complexity: 1, Depth: 2},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "node", Stats: OperationStats{FieldCount: 401, NodeCount: 1, Complexity: 1, Depth: 1}},
			},
		},
		{
			// A type definition with 20000 fields before 15 levels of fragments
			// that each spread the next one in two fields: 65534 expansions.
			name:       "type definition in the operation document",
			definition: fragmentTestDefinition,
			operation:  largeTypeDefinition(20000) + `{ me { ...Friends0 } }` + friendsFragmentLevels(15),
			stats:      OperationStats{FieldCount: 98303, NodeCount: 65535, Complexity: 65535, Depth: 17},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "me", Stats: OperationStats{FieldCount: 98303, NodeCount: 65535, Complexity: 65535, Depth: 16}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stats, rootFields := estimateInTime(t, tt.definition, tt.operation)
			assert.Equal(t, tt.stats, stats)
			assert.Equal(t, tt.rootFields, rootFields)
		})
	}
}

func TestCalculateOperationComplexityFragmentDepthLimit(t *testing.T) {
	t.Parallel()

	t.Run("fragments nested up to the limit", func(t *testing.T) {
		t.Parallel()

		operation := `{ me { ...Chain0 } }` + fragmentChain(maxFragmentDepth, "User", "id")
		stats, rootFields := estimateUnnormalized(t, fragmentTestDefinition, operation, false)
		assert.Equal(t, OperationStats{FieldCount: 2, NodeCount: 1, Complexity: 1, Depth: 2}, stats)
		assert.Equal(t, []RootFieldStats{
			{TypeName: "Query", FieldName: "me", Stats: OperationStats{FieldCount: 2, NodeCount: 1, Complexity: 1, Depth: 1}},
		}, rootFields)
	})

	t.Run("fragments nested beyond the limit", func(t *testing.T) {
		t.Parallel()

		definition := unsafeparser.ParseGraphqlDocumentString(fragmentTestDefinition)
		operation := unsafeparser.ParseGraphqlDocumentString(`{ me { ...Chain0 } }` + fragmentChain(maxFragmentDepth+1, "User", "id"))
		report := operationreport.Report{}
		NewOperationComplexityEstimator(false).Do(&operation, &definition, &report)

		assert.Empty(t, report.InternalErrors)
		require.Len(t, report.ExternalErrors, 1)
		assert.Equal(t, "fragment spread: Chain1000 exceeds the maximum fragment nesting depth of 1000", report.ExternalErrors[0].Message)
	})
}

func TestCalculateOperationComplexityFragmentCycles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		operation  string
		stats      OperationStats
		rootFields []RootFieldStats
	}{
		{
			name:      "fragment spreading itself",
			operation: `{ me { ...UserFields } } fragment UserFields on User { id ...UserFields }`,
			stats:     OperationStats{FieldCount: 2, NodeCount: 1, Complexity: 1, Depth: 2},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "me", Stats: OperationStats{FieldCount: 2, NodeCount: 1, Complexity: 1, Depth: 1}},
			},
		},
		{
			name: "fragments spreading each other",
			operation: `
				{ me { ...UserFields } }
				fragment UserFields on User { id ...NameFields }
				fragment NameFields on User { name ...UserFields }`,
			stats: OperationStats{FieldCount: 3, NodeCount: 1, Complexity: 1, Depth: 2},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "me", Stats: OperationStats{FieldCount: 3, NodeCount: 1, Complexity: 1, Depth: 1}},
			},
		},
		{
			name:      "fragment spreading itself in a nested field",
			operation: `{ me { ...UserFields } } fragment UserFields on User { id friends(first: 2) { ...UserFields } }`,
			stats:     OperationStats{FieldCount: 3, NodeCount: 3, Complexity: 2, Depth: 3},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "me", Stats: OperationStats{FieldCount: 3, NodeCount: 3, Complexity: 2, Depth: 2}},
			},
		},
		{
			name:      "root fragment spreading itself",
			operation: `{ ...RootFields } fragment RootFields on Query { currentPeriod self { ...RootFields } }`,
			stats:     OperationStats{FieldCount: 2, NodeCount: 1, Complexity: 1, Depth: 2},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "currentPeriod", Stats: OperationStats{FieldCount: 1}},
				{TypeName: "Query", FieldName: "self", Stats: OperationStats{FieldCount: 1, NodeCount: 1, Complexity: 1, Depth: 1}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stats, rootFields := estimateUnnormalized(t, fragmentTestDefinition, tt.operation, false)
			assert.Equal(t, tt.stats, stats)
			assert.Equal(t, tt.rootFields, rootFields)
		})
	}
}

func TestCalculateOperationComplexityUnresolvedFragmentSpreads(t *testing.T) {
	t.Parallel()

	t.Run("undefined fragment", func(t *testing.T) {
		t.Parallel()

		stats, rootFields := estimateUnnormalized(t, fragmentTestDefinition, `{ me { id ...Missing } }`, false)
		assert.Equal(t, OperationStats{FieldCount: 2, NodeCount: 1, Complexity: 1, Depth: 2}, stats)
		assert.Equal(t, []RootFieldStats{
			{TypeName: "Query", FieldName: "me", Stats: OperationStats{FieldCount: 2, NodeCount: 1, Complexity: 1, Depth: 1}},
		}, rootFields)
	})

	t.Run("undefined fragment next to a defined one", func(t *testing.T) {
		t.Parallel()

		stats, rootFields := estimateUnnormalized(t, fragmentTestDefinition, `{ me { ...Missing ...UserFields } } fragment UserFields on User { id }`, false)
		assert.Equal(t, OperationStats{FieldCount: 2, NodeCount: 1, Complexity: 1, Depth: 2}, stats)
		assert.Equal(t, []RootFieldStats{
			{TypeName: "Query", FieldName: "me", Stats: OperationStats{FieldCount: 2, NodeCount: 1, Complexity: 1, Depth: 1}},
		}, rootFields)
	})

	t.Run("fragment definition removed by normalization", func(t *testing.T) {
		t.Parallel()

		definition := unsafeparser.ParseGraphqlDocumentString(fragmentTestDefinition)
		operation := unsafeparser.ParseGraphqlDocumentString(`{ me { ...UserFields } } fragment UserFields on User { id name }`)
		// Normalization removes a fragment definition from the root nodes, but
		// keeps it in FragmentDefinitions.
		require.Equal(t, ast.NodeKindFragmentDefinition, operation.RootNodes[1].Kind)
		operation.RootNodes[1].Kind = ast.NodeKindUnknown

		report := operationreport.Report{}
		stats, rootFields := NewOperationComplexityEstimator(false).Do(&operation, &definition, &report)
		require.False(t, report.HasErrors(), report.Error())
		assert.Equal(t, OperationStats{FieldCount: 1, NodeCount: 1, Complexity: 1, Depth: 2}, stats)
		assert.Equal(t, []RootFieldStats{
			{TypeName: "Query", FieldName: "me", Stats: OperationStats{FieldCount: 1, NodeCount: 1, Complexity: 1, Depth: 1}},
		}, rootFields)
	})

	t.Run("fragment definition removed by normalization next to a live one", func(t *testing.T) {
		t.Parallel()

		definition := unsafeparser.ParseGraphqlDocumentString(fragmentTestDefinition)
		operation := unsafeparser.ParseGraphqlDocumentString(`
			{ me { ...RemovedFields ...UserFields } }
			fragment RemovedFields on User { id name address { city } }
			fragment UserFields on User { id }`)
		require.Equal(t, ast.NodeKindFragmentDefinition, operation.RootNodes[1].Kind)
		operation.RootNodes[1].Kind = ast.NodeKindUnknown

		report := operationreport.Report{}
		stats, rootFields := NewOperationComplexityEstimator(false).Do(&operation, &definition, &report)
		require.False(t, report.HasErrors(), report.Error())
		assert.Equal(t, OperationStats{FieldCount: 2, NodeCount: 1, Complexity: 1, Depth: 2}, stats)
		assert.Equal(t, []RootFieldStats{
			{TypeName: "Query", FieldName: "me", Stats: OperationStats{FieldCount: 2, NodeCount: 1, Complexity: 1, Depth: 1}},
		}, rootFields)
	})
}

func TestCalculateOperationComplexityFragmentSpreadsLeftByNormalization(t *testing.T) {
	t.Parallel()

	// Normalization does not inline a spread of a fragment on an unrelated
	// type, so the spread and its definition reach the estimator.
	definition := unsafeparser.ParseGraphqlDocumentString(fragmentTestDefinition)
	operation := unsafeparser.ParseGraphqlDocumentString(`{ me { id ...DogFields } } fragment DogFields on Dog { barks }`)
	report := operationreport.Report{}
	astnormalization.NormalizeOperation(&operation, &definition, &report)
	require.False(t, report.HasErrors(), report.Error())
	require.True(t, slices.ContainsFunc(operation.RootNodes, func(node ast.Node) bool {
		return node.Kind == ast.NodeKindFragmentDefinition
	}), "the normalized operation should keep the fragment definition")

	stats, rootFields := NewOperationComplexityEstimator(false).Do(&operation, &definition, &report)
	require.False(t, report.HasErrors(), report.Error())
	assert.Equal(t, OperationStats{FieldCount: 3, NodeCount: 1, Complexity: 1, Depth: 2}, stats)
	assert.Equal(t, []RootFieldStats{
		{TypeName: "Query", FieldName: "me", Stats: OperationStats{FieldCount: 3, NodeCount: 1, Complexity: 1, Depth: 1}},
	}, rootFields)

	inlinedStats, inlinedRootFields := estimateNormalized(t, fragmentTestDefinition, `{ me { id ... on Dog { barks } } }`, false)
	assert.Equal(t, inlinedStats, stats, "unexpected stats compared to inline fragments")
	assert.Equal(t, inlinedRootFields, rootFields, "unexpected root fields compared to inline fragments")
}

func TestCalculateOperationComplexityMergedFragmentSpreads(t *testing.T) {
	t.Parallel()

	// Repeated spreads are merged by scope and by the type they are spread on,
	// whatever the outer type conditions and the spreads' directives are. The
	// estimate is lower than for the inlined operation, which keeps a copy per
	// spread, but not lower than what field collection selects.
	tests := []struct {
		name       string
		operation  string
		stats      OperationStats
		rootFields []RootFieldStats
	}{
		{
			name: "same fragment through different type conditions",
			operation: `
				{ node(id: "1") { ...NodeFields } }
				fragment NodeFields on Node { ... on User { ...BranchFields } ... on Dog { ...BranchFields } }
				fragment BranchFields on Node { ... on User { ...IdFields } ... on Dog { ...IdFields } }
				fragment IdFields on Node { id }`,
			stats: OperationStats{FieldCount: 3, NodeCount: 1, Complexity: 1, Depth: 2},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "node", Stats: OperationStats{FieldCount: 3, NodeCount: 1, Complexity: 1, Depth: 1}},
			},
		},
		{
			name: "same fragment with different directives",
			operation: `
				query ($withUser: Boolean!) { me { ...UserFields @include(if: $withUser) ...UserFields @skip(if: $withUser) } }
				fragment UserFields on User { id }`,
			stats: OperationStats{FieldCount: 2, NodeCount: 1, Complexity: 1, Depth: 2},
			rootFields: []RootFieldStats{
				{TypeName: "Query", FieldName: "me", Stats: OperationStats{FieldCount: 2, NodeCount: 1, Complexity: 1, Depth: 1}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stats, rootFields := estimateUnnormalized(t, fragmentTestDefinition, tt.operation, false)
			assert.Equal(t, tt.stats, stats)
			assert.Equal(t, tt.rootFields, rootFields)
		})
	}
}

func TestOperationComplexityEstimatorReuseWithFragments(t *testing.T) {
	t.Parallel()

	definition := unsafeparser.ParseGraphqlDocumentString(fragmentTestDefinition)
	estimator := NewOperationComplexityEstimator(false)

	for _, operationString := range []string{
		`{ me { id } }`,
		`{ me { ...UserFields } } fragment UserFields on User { id friends(first: 2) { ...FriendFields } } fragment FriendFields on User { name }`,
		`{ me { id } }`,
		`{ me { ...UserFields } } fragment UserFields on User { address { city } }`,
		`{ ...RootFields } fragment RootFields on Query { currentPeriod me { id } }`,
		`{ me { ...UserFields } } fragment UserFields on User { id friends(first: 2) { ...FriendFields } } fragment FriendFields on User { name }`,
	} {
		operation := unsafeparser.ParseGraphqlDocumentString(operationString)

		wantReport := operationreport.Report{}
		wantStats, wantRootFields := NewOperationComplexityEstimator(false).Do(&operation, &definition, &wantReport)
		require.False(t, wantReport.HasErrors(), wantReport.Error())

		gotReport := operationreport.Report{}
		gotStats, gotRootFields := estimator.Do(&operation, &definition, &gotReport)
		require.False(t, gotReport.HasErrors(), gotReport.Error())

		assert.Equal(t, wantStats, gotStats, operationString)
		assert.Equal(t, wantRootFields, gotRootFields, operationString)
	}
}

func TestOperationComplexityEstimatorReuseAfterAbortedFragmentWalk(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		operation string
	}{
		{
			name:      "unknown type inside a fragment",
			operation: `{ users(first: 1) { ...UserFields } } fragment UserFields on User { friends(first: 1) { address { ... on UnknownType { city } } } }`,
		},
		{
			name:      "unknown fragment type condition",
			operation: `{ me { ...UserFields } } fragment UserFields on UnknownType { id }`,
		},
		{
			name: "unknown type inside a nested fragment",
			operation: `
				{ me { ...UserFields } }
				fragment UserFields on User { friends(first: 1) { ...FriendFields } }
				fragment FriendFields on User { address { ... on UnknownType { city } } }`,
		},
		{
			// The first aborted fragment walk aborts the whole walk, so the second
			// unknown type is not reported.
			name: "unknown types inside fragments of two fields",
			operation: `
				{ me { ...UserFields } user(id: "1") { ...OtherUserFields } }
				fragment UserFields on User { address { ... on UnknownType { city } } }
				fragment OtherUserFields on User { address { ... on OtherUnknownType { city } } }`,
		},
		{
			name:      "fragments nested beyond the limit",
			operation: `{ me { ...Chain0 } }` + fragmentChain(maxFragmentDepth+1, "User", "id"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			definition := unsafeparser.ParseGraphqlDocumentString(fragmentTestDefinition)
			estimator := NewOperationComplexityEstimator(false)

			invalidOperation := unsafeparser.ParseGraphqlDocumentString(tt.operation)
			invalidReport := operationreport.Report{}
			estimator.Do(&invalidOperation, &definition, &invalidReport)
			require.True(t, invalidReport.HasErrors())
			assert.Len(t, invalidReport.ExternalErrors, 1)

			operation := unsafeparser.ParseGraphqlDocumentString(`
				{ me { ...UserFields } }
				fragment UserFields on User { id friends(first: 2) { ...FriendFields } }
				fragment FriendFields on User { name address { city } }`)
			wantReport := operationreport.Report{}
			wantStats, wantRootFields := NewOperationComplexityEstimator(false).Do(&operation, &definition, &wantReport)
			require.False(t, wantReport.HasErrors(), wantReport.Error())

			gotReport := operationreport.Report{}
			gotStats, gotRootFields := estimator.Do(&operation, &definition, &gotReport)
			require.False(t, gotReport.HasErrors(), gotReport.Error())

			assert.Equal(t, OperationStats{FieldCount: 6, NodeCount: 5, Complexity: 4, Depth: 4}, gotStats)
			assert.Equal(t, wantStats, gotStats)
			assert.Equal(t, wantRootFields, gotRootFields)
		})
	}
}

func TestOperationComplexityEstimatorReuseForAbortedWalk(t *testing.T) {
	t.Parallel()

	// An aborted walk leaves its fields without restoring the enclosing types,
	// so a field can end as a root field without having started as one. Its
	// stats must not come from the previous operation.
	tests := []struct {
		name      string
		operation string
	}{
		{
			name:      "unknown type in an inline fragment",
			operation: `{ ... on User { viewer { me { ... on UnknownType { id } } } } }`,
		},
		{
			name:      "unknown fragment type condition",
			operation: `{ ... on User { viewer { ...UserFields } } } fragment UserFields on UnknownType { id }`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			definition := unsafeparser.ParseGraphqlDocumentString(fragmentTestDefinition)
			estimator := NewOperationComplexityEstimator(false)

			previousOperation := unsafeparser.ParseGraphqlDocumentString(`{ me { id name } }`)
			previousReport := operationreport.Report{}
			estimator.Do(&previousOperation, &definition, &previousReport)
			require.False(t, previousReport.HasErrors(), previousReport.Error())

			operation := unsafeparser.ParseGraphqlDocumentString(tt.operation)
			wantReport := operationreport.Report{}
			wantStats, wantRootFields := NewOperationComplexityEstimator(false).Do(&operation, &definition, &wantReport)
			require.True(t, wantReport.HasErrors())

			gotReport := operationreport.Report{}
			gotStats, gotRootFields := estimator.Do(&operation, &definition, &gotReport)
			require.True(t, gotReport.HasErrors())

			assert.Equal(t, wantStats, gotStats)
			assert.Equal(t, wantRootFields, gotRootFields)
		})
	}
}

// estimateUnnormalized estimates the operation as parsed. Unlike run, it does
// not normalize the operation first, so fragment spreads reach the estimator.
func estimateUnnormalized(t *testing.T, definition, operation string, skipIntrospection bool) (OperationStats, []RootFieldStats) {
	t.Helper()

	def := unsafeparser.ParseGraphqlDocumentString(definition)
	op := unsafeparser.ParseGraphqlDocumentString(operation)
	report := operationreport.Report{}

	stats, rootFields := NewOperationComplexityEstimator(skipIntrospection).Do(&op, &def, &report)
	require.False(t, report.HasErrors(), report.Error())
	return stats, rootFields
}

// estimateNormalized estimates the operation after normalizing it, which
// inlines its fragment spreads.
func estimateNormalized(t *testing.T, definition, operation string, skipIntrospection bool) (OperationStats, []RootFieldStats) {
	t.Helper()

	def := unsafeparser.ParseGraphqlDocumentString(definition)
	op := unsafeparser.ParseGraphqlDocumentString(operation)
	report := operationreport.Report{}

	astnormalization.NormalizeOperation(&op, &def, &report)
	require.False(t, report.HasErrors(), report.Error())

	stats, rootFields := NewOperationComplexityEstimator(skipIntrospection).Do(&op, &def, &report)
	require.False(t, report.HasErrors(), report.Error())
	return stats, rootFields
}

// fragmentSpreadBomb returns fragment definitions Bomb0 to Bomb<levels>, where
// each fragment spreads the next one twice and the last one selects leaf.
func fragmentSpreadBomb(levels int, typeName, leaf string) string {
	var fragments strings.Builder
	for i := range levels {
		fmt.Fprintf(&fragments, "\nfragment Bomb%d on %s { ...Bomb%d ...Bomb%d }", i, typeName, i+1, i+1)
	}
	fmt.Fprintf(&fragments, "\nfragment Bomb%d on %s { %s }", levels, typeName, leaf)
	return fragments.String()
}

// typeConditionFragmentSpreadBomb returns fragment definitions Bomb0 to
// Bomb<levels> on Node, where each fragment spreads the next one in an inline
// fragment on User and in one on Dog, and the last one selects id.
func typeConditionFragmentSpreadBomb(levels int) string {
	var fragments strings.Builder
	for i := range levels {
		fmt.Fprintf(&fragments, "\nfragment Bomb%d on Node { ... on User { ...Bomb%d } ... on Dog { ...Bomb%d } }", i, i+1, i+1)
	}
	fmt.Fprintf(&fragments, "\nfragment Bomb%d on Node { id }", levels)
	return fragments.String()
}

// fragmentChain returns fragment definitions Chain0 to Chain<count-1>, where
// each fragment spreads the next one and the last one selects leaf.
func fragmentChain(count int, typeName, leaf string) string {
	var fragments strings.Builder
	for i := range count - 1 {
		fmt.Fprintf(&fragments, "\nfragment Chain%d on %s { ...Chain%d }", i, typeName, i+1)
	}
	fmt.Fprintf(&fragments, "\nfragment Chain%d on %s { %s }", count-1, typeName, leaf)
	return fragments.String()
}

// implementationsDefinition returns a schema where Query.node returns the
// interface Node, which the types T0 to T<types-1> implement.
func implementationsDefinition(types int) string {
	var definition strings.Builder
	definition.WriteString("scalar ID\nschema { query: Query }\ntype Query { node: Node }\ninterface Node { id: ID! }")
	for i := range types {
		fmt.Fprintf(&definition, "\ntype T%d implements Node { id: ID! }", i)
	}
	return definition.String()
}

// typeConditionFragmentLevels returns fragment definitions Level<i>_<j> on
// T<j> for i up to levels and j below types. Each fragment spreads every
// fragment of the next level, and the fragments of the last level select id.
func typeConditionFragmentLevels(levels, types int) string {
	var fragments strings.Builder
	for i := range levels {
		for j := range types {
			fmt.Fprintf(&fragments, "\nfragment Level%d_%d on T%d {", i, j, j)
			for k := range types {
				fmt.Fprintf(&fragments, " ...Level%d_%d", i+1, k)
			}
			fragments.WriteString(" }")
		}
	}
	for j := range types {
		fmt.Fprintf(&fragments, "\nfragment Level%d_%d on T%d { id }", levels, j, j)
	}
	return fragments.String()
}

// largeTypeDefinition returns an object type definition with the given number
// of fields, each with an argument and a directive.
func largeTypeDefinition(fields int) string {
	var definition strings.Builder
	definition.WriteString("type Large {")
	for i := range fields {
		fmt.Fprintf(&definition, " field%d(arg: Int @deprecated(reason: \"none\")): String", i)
	}
	definition.WriteString(" }\n")
	return definition.String()
}

// friendsFragmentLevels returns fragment definitions Friends0 to
// Friends<levels> on User, where each fragment spreads the next one in two
// friends fields and the last one selects id.
func friendsFragmentLevels(levels int) string {
	var fragments strings.Builder
	for i := range levels {
		fmt.Fprintf(&fragments, "\nfragment Friends%d on User { friends(first: 1) { ...Friends%d } others: friends(first: 1) { ...Friends%d } }", i, i+1, i+1)
	}
	fmt.Fprintf(&fragments, "\nfragment Friends%d on User { id }", levels)
	return fragments.String()
}

// estimateInTime estimates the operation as parsed and fails the test if that
// takes longer than 10 seconds.
func estimateInTime(t *testing.T, definition, operation string) (OperationStats, []RootFieldStats) {
	t.Helper()

	def := unsafeparser.ParseGraphqlDocumentString(definition)
	op := unsafeparser.ParseGraphqlDocumentString(operation)

	type result struct {
		stats      OperationStats
		rootFields []RootFieldStats
		report     operationreport.Report
	}
	done := make(chan result, 1)
	go func() {
		report := operationreport.Report{}
		stats, rootFields := NewOperationComplexityEstimator(false).Do(&op, &def, &report)
		done <- result{stats: stats, rootFields: rootFields, report: report}
	}()

	select {
	case got := <-done:
		require.False(t, got.report.HasErrors(), got.report.Error())
		return got.stats, got.rootFields
	case <-time.After(10 * time.Second):
		t.Fatal("estimating the operation did not finish in time")
		return OperationStats{}, nil
	}
}

func runConfig(t *testing.T, definition, operation string, expectedGlobalComplexityResult OperationStats, expectedFieldsComplexityResult []RootFieldStats, skipIntrospection bool) {
	def := unsafeparser.ParseGraphqlDocumentString(definition)
	op := unsafeparser.ParseGraphqlDocumentString(operation)
	report := operationreport.Report{}

	astnormalization.NormalizeOperation(&op, &def, &report)

	estimator := NewOperationComplexityEstimator(skipIntrospection)
	actualGlobalComplexityResult, actualFieldsComplexityResult := estimator.Do(&op, &def, &report)
	require.False(t, report.HasErrors())

	assert.Equal(t, expectedGlobalComplexityResult.FieldCount, actualGlobalComplexityResult.FieldCount, "unexpected global field count")
	assert.Equal(t, expectedGlobalComplexityResult.NodeCount, actualGlobalComplexityResult.NodeCount, "unexpected global node count")
	assert.Equal(t, expectedGlobalComplexityResult.Complexity, actualGlobalComplexityResult.Complexity, "unexpected global complexity")
	assert.Equal(t, expectedGlobalComplexityResult.Depth, actualGlobalComplexityResult.Depth, "unexpected global depth")
	assert.Equal(t, expectedFieldsComplexityResult, actualFieldsComplexityResult, "unexpected fields complexity result")
}

func run(t *testing.T, definition, operation string, expectedGlobalComplexityResult OperationStats, expectedFieldsComplexityResult []RootFieldStats) {
	runConfig(t, definition, operation, expectedGlobalComplexityResult, expectedFieldsComplexityResult, false)
}

func runSkipIntrospection(t *testing.T, definition, operation string, expectedGlobalComplexityResult OperationStats, expectedFieldsComplexityResult []RootFieldStats) {
	runConfig(t, definition, operation, expectedGlobalComplexityResult, expectedFieldsComplexityResult, true)
}

func BenchmarkEstimateComplexity(b *testing.B) {
	def := unsafeparser.ParseGraphqlDocumentString(testDefinition)
	op := unsafeparser.ParseGraphqlDocumentString(complexQuery)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// We use NewOperationComplexityEstimator for every operation in production, thus
		// we want it in the benchmarking loop.
		estimator := NewOperationComplexityEstimator(false)
		report := operationreport.Report{}
		globalComplexityResult, _ := estimator.Do(&op, &def, &report)
		if report.HasErrors() {
			b.Fatal(report)
		}

		if globalComplexityResult.NodeCount != 920 {
			b.Fatalf("want nodeCount: 920, got: %d\n", globalComplexityResult.NodeCount)
		}
		if globalComplexityResult.Complexity != 221 {
			b.Fatalf("want complexity: 221, got: %d\n", globalComplexityResult.Complexity)
		}
		if globalComplexityResult.Depth != 5 {
			b.Fatalf("want depth: 5, got: %d\n", globalComplexityResult.Depth)
		}
	}
}

func BenchmarkEstimateComplexityWithFragments(b *testing.B) {
	def := unsafeparser.ParseGraphqlDocumentString(testDefinition)
	op := unsafeparser.ParseGraphqlDocumentString(complexQueryWithFragments)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		estimator := NewOperationComplexityEstimator(false)
		report := operationreport.Report{}
		globalComplexityResult, _ := estimator.Do(&op, &def, &report)
		if report.HasErrors() {
			b.Fatal(report)
		}

		if globalComplexityResult.NodeCount != 920 {
			b.Fatalf("want nodeCount: 920, got: %d\n", globalComplexityResult.NodeCount)
		}
		if globalComplexityResult.Complexity != 221 {
			b.Fatalf("want complexity: 221, got: %d\n", globalComplexityResult.Complexity)
		}
		if globalComplexityResult.Depth != 5 {
			b.Fatalf("want depth: 5, got: %d\n", globalComplexityResult.Depth)
		}
	}
}

const complexQuery = `
{
  users(first: 10) {
	id
	balance
	name
	address {
	  city
	  country
	}
	transactions(first: 5) {
		id
		amount
		sender {
			id
			transactions(first: 10) {
				id
				amount
			}
		}
		recipient {
			id
			transactions(first: 5) {
				id
				amount
			}
		}
	}
  }
}`

// complexQueryWithFragments is complexQuery with some of its selections moved
// into fragments.
const complexQueryWithFragments = `
{
  users(first: 10) {
	...UserFields
  }
}

fragment UserFields on User {
	id
	balance
	name
	address {
	  ...AddressFields
	}
	transactions(first: 5) {
		...TransactionFields
		sender {
			id
			transactions(first: 10) {
				...TransactionFields
			}
		}
		recipient {
			id
			transactions(first: 5) {
				...TransactionFields
			}
		}
	}
}

fragment AddressFields on Address {
	city
	country
}

fragment TransactionFields on Transaction {
	id
	amount
}`

const fragmentTestDefinition = `
directive @nodeCountMultiply on ARGUMENT_DEFINITION
directive @nodeCountSkip on FIELD

scalar ID
scalar Int
scalar String
scalar Boolean

schema {
	query: Query
	mutation: Mutation
}

type Query {
	me: User
	user(id: ID!): User
	users(first: Int! @nodeCountMultiply): [User]
	node(id: ID!): Node
	search(first: Int! @nodeCountMultiply): [SearchResult]
	self: Query
	currentPeriod: String
	hidden: User @nodeCountSkip
}

type Mutation {
	createUser(name: String!): User
	deleteUser(id: ID!): User
}

interface Node {
	id: ID!
}

interface Pet {
	name: String!
	owner: User
}

type User implements Node {
	id: ID!
	name: String!
	secret: String @nodeCountSkip
	address: Address
	friends(first: Int! @nodeCountMultiply): [User]
	pets: [Pet]
	viewer: Query
}

type Address {
	city: String
	country: String
}

type Dog implements Node & Pet {
	id: ID!
	name: String!
	owner: User
	barks: Boolean
}

type Cat implements Node & Pet {
	id: ID!
	name: String!
	owner: User
	meows: Boolean
}

union SearchResult = User | Dog | Cat
`

const depthRegressionDefinition = `
directive @nodeCountMultiply on ARGUMENT_DEFINITION

scalar String

schema { query: Query }

input RootInput { key: String }
type Query {
  root(input: RootInput, first: Int @nodeCountMultiply): Node
}

interface Node {
  next(input: RootInput, first: Int @nodeCountMultiply): Node
  leaf(input: RootInput): String
  branchA: Node
  branchB: Node
}

type Concrete implements Node {
  next(input: RootInput, first: Int @nodeCountMultiply): Node
  leaf(input: RootInput): String
  branchA: Node
  branchB: Node
}
`

const testDefinition = `

directive @nodeCountMultiply on ARGUMENT_DEFINITION
directive @nodeCountSkip on FIELD

scalar Date

type User {
    id: ID!
    balance: Int!
    name: String!
    email: String!
    address: Address
    transactions(first: Int! @nodeCountMultiply, afterID: ID): [Transaction]
}

type Address {
    street: String
    city: String
    postalCode: String
    country: String
}

type Transaction {
    id: ID!
    date: Date!
    amount: Int!
    sender: User!
    recipient: User!
}

input NewTransaction {
    sender: ID!
    recipient: ID!
    amount: Int!
}

input AddressInput {
    street: String!
    city: String!
    postalCode: String!
    country: String!
}

input UpdateUserDetailsInput {
    name: String
    address: AddressInput
}

input NewUserInput {
    balance: Int!
    name: String!
    email: String!
}

type Query {
	__schema: __Schema!
    user(id: ID!): User
    users(first: Int! @nodeCountMultiply, afterID: ID): [User]
    transactions(first: Int! @nodeCountMultiply, afterID: ID): [Transaction]
    currentPeriod: String
	activeUsers: [User] @nodeCountSkip
}

type Mutation {
    createUser(input: NewUserInput!): User
    makeTransaction(input: NewTransaction!): Transaction!
    updateUserDetails(userID: ID!,input: UpdateUserDetailsInput!): User
}

schema {
	query: Query
	mutation: Mutation
}

"The 'Int' scalar type represents non-fractional signed whole numeric values. Int can represent values between -(2^31) and 2^31 - 1."
scalar Int
"The 'Float' scalar type represents signed double-precision fractional values as specified by [IEEE 754](http://en.wikipedia.org/wiki/IEEE_floating_point)."
scalar Float
"The 'String' scalar type represents textual data, represented as UTF-8 character sequences. The String type is most often used by GraphQL to represent free-form human-readable text."
scalar String
"The 'Boolean' scalar type represents 'true' or 'false' ."
scalar Boolean
"The 'ID' scalar type represents a unique identifier, often used to refetch an object or as key for a cache. The ID type appears in a JSON response as a String; however, it is not intended to be human-readable. When expected as an input type, any string (such as '4') or integer (such as 4) input value will be accepted as an ID."
scalar ID @custom(typeName: "string")
"Directs the executor to include this field or fragment only when the argument is true."
directive @include(
    "Included when true."
    if: Boolean!
) on FIELD | FRAGMENT_SPREAD | INLINE_FRAGMENT
"Directs the executor to skip this field or fragment when the argument is true."
directive @skip(
    "Skipped when true."
    if: Boolean!
) on FIELD | FRAGMENT_SPREAD | INLINE_FRAGMENT
"Marks an element of a GraphQL schema as no longer supported."
directive @deprecated(
    """
    Explains why this element was deprecated, usually also including a suggestion
    for how to access supported similar data. Formatted in
    [Markdown](https://daringfireball.net/projects/markdown/).
    """
    reason: String = "No longer supported"
) on FIELD_DEFINITION | ENUM_VALUE

"""
A Directive provides a way to describe alternate runtime execution and type validation behavior in a GraphQL document.
In some cases, you need to provide options to alter GraphQL's execution behavior
in ways field arguments will not suffice, such as conditionally including or
skipping a field. Directives provide this by describing additional information
to the executor.
"""
type __Directive {
    name: String!
    description: String
    locations: [__DirectiveLocation!]!
    args: [__InputValue!]!
}

"""
A Directive can be adjacent to many parts of the GraphQL language, a
__DirectiveLocation describes one such possible adjacencies.
"""
enum __DirectiveLocation {
    "Location adjacent to a query operation."
    QUERY
    "Location adjacent to a mutation operation."
    MUTATION
    "Location adjacent to a subscription operation."
    SUBSCRIPTION
    "Location adjacent to a field."
    FIELD
    "Location adjacent to a fragment definition."
    FRAGMENT_DEFINITION
    "Location adjacent to a fragment spread."
    FRAGMENT_SPREAD
    "Location adjacent to an inline fragment."
    INLINE_FRAGMENT
    "Location adjacent to a schema definition."
    SCHEMA
    "Location adjacent to a scalar definition."
    SCALAR
    "Location adjacent to an object type definition."
    OBJECT
    "Location adjacent to a field definition."
    FIELD_DEFINITION
    "Location adjacent to an argument definition."
    ARGUMENT_DEFINITION
    "Location adjacent to an interface definition."
    INTERFACE
    "Location adjacent to a union definition."
    UNION
    "Location adjacent to an enum definition."
    ENUM
    "Location adjacent to an enum value definition."
    ENUM_VALUE
    "Location adjacent to an input object type definition."
    INPUT_OBJECT
    "Location adjacent to an input object field definition."
    INPUT_FIELD_DEFINITION
}
"""
One possible value for a given Enum. Enum values are unique values, not a
placeholder for a string or numeric value. However an Enum value is returned in
a JSON response as a string.
"""
type __EnumValue {
    name: String!
    description: String
    isDeprecated: Boolean!
    deprecationReason: String
}

"""
Object and Interface types are described by a list of Fields, each of which has
a name, potentially a list of arguments, and a return type.
"""
type __Field {
    name: String!
    description: String
    args: [__InputValue!]!
    type: __Type!
    isDeprecated: Boolean!
    deprecationReason: String
}

"""Arguments provided to Fields or Directives and the input fields of an
InputObject are represented as Input Values which describe their type and
optionally a default value.
"""
type __InputValue {
    name: String!
    description: String
    type: __Type!
    "A GraphQL-formatted string representing the default value for this input value."
    defaultValue: String
}

"""
A GraphQL Schema defines the capabilities of a GraphQL server. It exposes all
available types and directives on the server, as well as the entry points for
query, mutation, and subscription operations.
"""
type __Schema {
    "A list of all types supported by this server."
    types: [__Type!]!
    "The type that query operations will be rooted at."
    queryType: __Type!
    "If this server supports mutation, the type that mutation operations will be rooted at."
    mutationType: __Type
    "If this server support subscription, the type that subscription operations will be rooted at."
    subscriptionType: __Type
    "A list of all directives supported by this server."
    directives: [__Directive!]!
}

"""
The fundamental unit of any GraphQL Schema is the type. There are many kinds of
types in GraphQL as represented by the '__TypeKind' enum.

Depending on the kind of a type, certain fields describe information about that
type. Scalar types provide no information beyond a name and description, while
Enum types provide their values. Object and Interface types provide the fields
they describe. Abstract types, Union and Interface, provide the Object types
possible at runtime. List and NonNull types compose other types.
"""
type __Type {
    kind: __TypeKind!
    name: String
    description: String
    fields(includeDeprecated: Boolean = false): [__Field!]
    interfaces: [__Type!]
    possibleTypes: [__Type!]
    enumValues(includeDeprecated: Boolean = false): [__EnumValue!]
    inputFields: [__InputValue!]
    ofType: __Type
}

"An enum describing what kind of type a given '__Type' is."
enum __TypeKind {
    "Indicates this type is a scalar."
    SCALAR
    "Indicates this type is an object. 'fields' and 'interfaces' are valid fields."
    OBJECT
    "Indicates this type is an interface. 'fields' ' and ' 'possibleTypes' are valid fields."
    INTERFACE
    "Indicates this type is a union. 'possibleTypes' is a valid field."
    UNION
    "Indicates this type is an enum. 'enumValues' is a valid field."
    ENUM
    "Indicates this type is an input object. 'inputFields' is a valid field."
    INPUT_OBJECT
    "Indicates this type is a list. 'ofType' is a valid field."
    LIST
    "Indicates this type is a non-null. 'ofType' is a valid field."
    NON_NULL
}`

const introspectionQuery = `
query IntrospectionQuery {
  __schema {
    queryType {
      name
    }
    mutationType {
      name
    }
    subscriptionType {
      name
    }
    types {
      ...FullType
    }
    directives {
      name
      description
      locations
      args {
        ...InputValue
      }
    }
  }
}

fragment FullType on __Type {
  kind
  name
  description
  fields(includeDeprecated: true) {
    name
    description
    args {
      ...InputValue
    }
    type {
      ...TypeRef
    }
    isDeprecated
    deprecationReason
  }
  inputFields {
    ...InputValue
  }
  interfaces {
    ...TypeRef
  }
  enumValues(includeDeprecated: true) {
    name
    description
    isDeprecated
    deprecationReason
  }
  possibleTypes {
    ...TypeRef
  }
}

fragment InputValue on __InputValue {
  name
  description
  type {
    ...TypeRef
  }
  defaultValue
}

fragment TypeRef on __Type {
  kind
  name
  ofType {
    kind
    name
    ofType {
      kind
      name
      ofType {
        kind
        name
        ofType {
          kind
          name
          ofType {
            kind
            name
            ofType {
              kind
              name
              ofType {
                kind
                name
              }
            }
          }
        }
      }
    }
  }
}`
