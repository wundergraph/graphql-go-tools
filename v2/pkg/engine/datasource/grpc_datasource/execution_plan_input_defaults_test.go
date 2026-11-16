package grpcdatasource

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExecutionPlan_InputFieldDefaults(t *testing.T) {
	t.Parallel()

	// Clients may omit an input object field that has a default. A gRPC subgraph
	// has no GraphQL layer to apply it, so the plan has to carry it.
	schema := `
			type Query {
				items(sort: SortInput!): [Item!]!
			}

			type Item {
				id: ID!
			}

			enum SortOrder {
				ASC
				DESC
			}

			input DateRange {
				start: String!
				end: String!
			}

			input SortInput {
				field: String!
				order: SortOrder! = DESC
				limit: Int! = 10
				tags: [String!] = ["a", "b"]
				dateRange: DateRange! = { start: "2026-01-01", end: "2026-01-01" }
			}`

	mapping := &GRPCMapping{
		Service: "Items",
		QueryRPCs: map[string]RPCConfig{
			"items": {RPC: "QueryItems", Request: "QueryItemsRequest", Response: "QueryItemsResponse"},
		},
	}

	plan := planRecursiveTest(t, `query Items($sort: SortInput!) { items(sort: $sort) { id } }`, schema, mapping)

	require.Len(t, plan.Calls, 1)
	sort := findField(t, plan.Calls[0].Request.Fields, "sort")
	require.NotNil(t, sort.Message)

	require.Equal(t, "", findField(t, sort.Message.Fields, "field").DefaultValue)
	require.Equal(t, `"DESC"`, findField(t, sort.Message.Fields, "order").DefaultValue)
	require.Equal(t, `10`, findField(t, sort.Message.Fields, "limit").DefaultValue)
	require.Equal(t, `["a","b"]`, findField(t, sort.Message.Fields, "tags").DefaultValue)

	// The default of an input object field is the JSON object. Its nested message
	// is planned as usual, so the default is encoded like a value the client sent.
	dateRange := findField(t, sort.Message.Fields, "dateRange")
	require.Equal(t, `{"start":"2026-01-01","end":"2026-01-01"}`, dateRange.DefaultValue)
	require.NotNil(t, dateRange.Message)
	require.Equal(t, "DateRange", dateRange.Message.Name)
	require.Equal(t, "", findField(t, dateRange.Message.Fields, "start").DefaultValue)
	require.Equal(t, "", findField(t, dateRange.Message.Fields, "end").DefaultValue)
}
