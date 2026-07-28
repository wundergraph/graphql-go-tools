package resolve

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func multiEntityTestNode() *FetchTreeNode {
	return &FetchTreeNode{
		Kind: FetchTreeNodeKindSingle,
		Item: &FetchItem{
			Fetch: &MultiEntityFetch{
				FetchDependencies: FetchDependencies{
					FetchID:           1,
					DependsOnFetchIDs: []int{0},
				},
				Info: &FetchInfo{
					DataSourceID:   "products-id",
					DataSourceName: "products",
					QueryPlan: &QueryPlan{
						Query: "query {...}",
						DependsOnFields: []Representation{
							{
								Kind:     RepresentationKindKey,
								TypeName: "Product",
								Fragment: "... on Product {\n    __typename\n    upc\n}",
							},
						},
					},
					CoordinateDependencies: []FetchDependency{
						{
							Coordinate:      GraphCoordinate{TypeName: "Product", FieldName: "name"},
							IsUserRequested: true,
							DependsOn: []FetchDependencyOrigin{
								{
									FetchID:    0,
									Subgraph:   "products",
									Coordinate: GraphCoordinate{TypeName: "Product", FieldName: "upc"},
									IsKey:      true,
								},
							},
						},
					},
				},
				MergedFetchIDs: []int{1, 2},
				Input: MultiEntityInput{
					Entries: []MultiEntityFetchEntry{
						{Alias: "f1", Item: &FetchItem{ResponsePath: "employees.products"}},
						{Alias: "f2", Item: &FetchItem{ResponsePath: "employee"}},
					},
				},
			},
		},
	}
}

func TestFetchTreeNode_Trace_MultiEntity(t *testing.T) {
	node := multiEntityTestNode()
	data, err := json.Marshal(node.Trace())
	assert.NoError(t, err)

	// Full-document equality: every populated field of the trace node.
	expected := `{
		"kind": "Single",
		"fetch": {
			"kind": "MultiEntity",
			"path": "",
			"source_id": "products-id",
			"source_name": "products",
			"entries": [
				{"alias": "f1", "path": "employees.products"},
				{"alias": "f2", "path": "employee"}
			]
		}
	}`
	assert.JSONEq(t, expected, string(data))
}

func TestFetchTreeNode_QueryPlan_MultiEntity(t *testing.T) {
	node := multiEntityTestNode()
	data, err := json.Marshal(node.QueryPlan())
	assert.NoError(t, err)

	// Full-document equality: every populated field of the query-plan node.
	expected := `{
		"version": "1",
		"kind": "Single",
		"fetch": {
			"kind": "MultiEntity",
			"subgraphName": "products",
			"subgraphId": "products-id",
			"fetchId": 1,
			"dependsOnFetchIds": [0],
			"representations": [
				{
					"kind": "@key",
					"typeName": "Product",
					"fragment": "... on Product {\n    __typename\n    upc\n}"
				}
			],
			"query": "query {...}",
			"dependencies": [
				{
					"coordinate": {"typeName": "Product", "fieldName": "name"},
					"isUserRequested": true,
					"dependsOn": [
						{
							"fetchId": 0,
							"subgraph": "products",
							"coordinate": {"typeName": "Product", "fieldName": "upc"},
							"isKey": true,
							"isRequires": false
						}
					]
				}
			],
			"mergedFetchIds": [1, 2],
			"entries": [
				{"alias": "f1", "path": "employees.products"},
				{"alias": "f2", "path": "employee"}
			]
		}
	}`
	assert.JSONEq(t, expected, string(data))
}

func TestFetchTreeQueryPlanNode_PrettyPrint_Trigger(t *testing.T) {
	t.Run("just a trigger", func(t *testing.T) {
		fetches := Sequence()
		fetches.Trigger = &FetchTreeNode{
			Kind: FetchTreeNodeKindTrigger,
			Item: &FetchItem{
				Fetch: &SingleFetch{
					FetchDependencies: FetchDependencies{
						FetchID: 0,
					},
					Info: &FetchInfo{
						DataSourceID:   "0",
						DataSourceName: "country",
						QueryPlan: &QueryPlan{
							Query: `subscription {
    countryUpdated {
        name
    }
}`,
						},
					},
				},
				ResponsePath: "countryUpdated",
			},
		}

		queryPlan := fetches.QueryPlan()
		actual := queryPlan.PrettyPrint()

		expected := `
QueryPlan {
  Subscription {
    Primary: {
      Fetch(service: "country") {
        {
            countryUpdated {
                name
            }
        }
      }
    },
  }
}`

		assert.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(actual))
	})
	t.Run("trigger with representation call", func(t *testing.T) {
		fetches := Sequence()
		fetches.Trigger = &FetchTreeNode{
			Kind: FetchTreeNodeKindTrigger,
			Item: &FetchItem{
				Fetch: &SingleFetch{
					FetchDependencies: FetchDependencies{
						FetchID: 0,
					},
					Info: &FetchInfo{
						DataSourceID:   "0",
						DataSourceName: "country",
						QueryPlan: &QueryPlan{
							Query: `subscription {
    countryUpdated {
        name
        time {
            local
        }
    }
}`,
						},
					},
				},
				ResponsePath: "countryUpdated",
			},
		}
		fetches.ChildNodes = []*FetchTreeNode{{
			Kind: FetchTreeNodeKindSingle,
			Item: &FetchItem{
				Fetch: &SingleFetch{
					FetchDependencies: FetchDependencies{
						FetchID:           1,
						DependsOnFetchIDs: []int{0},
					},
					Info: &FetchInfo{
						DataSourceID:   "1",
						DataSourceName: "time",
						OperationType:  ast.OperationTypeQuery,
						QueryPlan: &QueryPlan{
							Query: "query($representations: [_Any!]!){\n    _entities(representations: $representations){\n        ... on Time {\n            __typename\n            local\n        }\n    }\n}",
						},
					},
				},
				ResponsePath: "countryUpdated.time",
			},
		}}

		queryPlan := fetches.QueryPlan()
		actual := queryPlan.PrettyPrint()

		expected := `
QueryPlan {
  Subscription {
    Primary: {
      Fetch(service: "country") {
        {
            countryUpdated {
                name
                time {
                    local
                }
            }
        }
      }
    },
    Rest: {
      Flatten(path: "countryUpdated.time") {
        Fetch(service: "time") {
          {
              _entities(representations: $representations){
                  ... on Time {
                      __typename
                      local
                  }
              }
          }
        }
      }
    },
  }
}`

		assert.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(actual))
	})
}

func TestFetchTreeQueryPlanNode_PrettyPrint_MultiEntity(t *testing.T) {
	fetches := Sequence()
	fetches.ChildNodes = []*FetchTreeNode{{
		Kind: FetchTreeNodeKindSingle,
		Item: &FetchItem{
			Fetch: &MultiEntityFetch{
				FetchDependencies: FetchDependencies{
					FetchID:           1,
					DependsOnFetchIDs: []int{0},
				},
				Info: &FetchInfo{
					DataSourceID:   "products-id",
					DataSourceName: "products",
					QueryPlan: &QueryPlan{
						Query: `query($representations_f1: [_Any!]!, $representations_f2: [_Any!]!, $includeF1: Boolean!, $includeF2: Boolean!){
    f1: _entities(representations: $representations_f1) @include(if: $includeF1){
        ... on Product {
            __typename
            name
        }
    }
    f2: _entities(representations: $representations_f2) @include(if: $includeF2){
        ... on Product {
            __typename
            price
        }
    }
}`,
						DependsOnFields: []Representation{
							{
								Kind:     RepresentationKindKey,
								TypeName: "Product",
								Fragment: "... on Product {\n    __typename\n    upc\n}",
							},
							{
								Kind:     RepresentationKindKey,
								TypeName: "Product",
								Fragment: "... on Product {\n    __typename\n    upc\n}",
							},
						},
					},
				},
				MergedFetchIDs: []int{1, 2},
				Input: MultiEntityInput{
					Entries: []MultiEntityFetchEntry{
						{Alias: "f1", Item: &FetchItem{ResponsePath: "employees.products"}},
						{Alias: "f2", Item: &FetchItem{ResponsePath: "employee"}},
					},
				},
			},
		},
	}}

	queryPlan := fetches.QueryPlan()
	actual := queryPlan.PrettyPrint()

	expected := `
QueryPlan {
  Fetch(service: "products") {
    {
      ... on Product {
          __typename
          upc
      }
      ... on Product {
          __typename
          upc
      }
    } =>
    {
        f1: _entities(representations: $representations_f1) @include(if: $includeF1){
            ... on Product {
                __typename
                name
            }
        }
        f2: _entities(representations: $representations_f2) @include(if: $includeF2){
            ... on Product {
                __typename
                price
            }
        }
    }
  }
}`

	assert.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(actual))
}
