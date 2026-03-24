package resolve

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

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
