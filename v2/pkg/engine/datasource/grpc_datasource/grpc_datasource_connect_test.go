package grpcdatasource

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
)

// Test_DataSource_loadWithConnect_ParityWithGRPC drives loadWithConnect — the
// proto-message based load path that the Connect client will use — against the
// same in-process gRPC server used by the rest of this package's tests. For
// each scenario both load paths are executed against an identical input and
// their JSON outputs are required to match, which validates that
// createProtoMessage (and the supporting wire_proto.go builders) produce
// semantically equivalent requests to the wire-bytes path that ships today.
//
// Until the Connect transport lands, the existing in-process gRPC client
// satisfies RPCTransport for both paths and is enough to exercise the proto
// message builders end-to-end.
func Test_DataSource_loadWithConnect_ParityWithGRPC(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	testCases := []struct {
		name              string
		query             string
		vars              string
		federationConfigs plan.FederationFieldConfigurations
	}{
		{
			name:  "scalar input with nested struct",
			query: `query ComplexFilterTypeQuery($filter: ComplexFilterTypeInput!) { complexFilterType(filter: $filter) { id name } }`,
			vars:  `{"variables":{"filter":{"filter":{"name":"Test Product","filterField1":"filterField1","filterField2":"filterField2"}}}}`,
		},
		{
			name:  "list result with no variables",
			query: `query { categories { id name kind } }`,
			vars:  `{}`,
		},
		{
			name:  "enum variable",
			query: `query($kind: CategoryKind!) { categoriesByKind(kind: $kind) { id name kind } }`,
			vars:  `{"variables":{"kind":"FURNITURE"}}`,
		},
		{
			name:  "nested input with pagination",
			query: `query($filter: CategoryFilter!) { filterCategories(filter: $filter) { id name kind } }`,
			vars:  `{"variables":{"filter":{"category":"ELECTRONICS","pagination":{"page":1,"perPage":2}}}}`,
		},
		{
			name: "deeply nested input lists",
			query: `query CalculateTotals($orders: [OrderInput!]!) {
				calculateTotals(orders: $orders) {
					orderId customerName totalItems
					orderLines { productId quantity modifiers }
				}
			}`,
			vars: `{"variables":{"orders":[
				{"orderId":"order-1","customerName":"John Doe","lines":[
					{"productId":"product-1","quantity":3,"modifiers":["discount-10"]},
					{"productId":"product-2","quantity":2,"modifiers":["tax-20"]}
				]},
				{"orderId":"order-2","customerName":"Jane Smith","lines":[
					{"productId":"product-3","quantity":1,"modifiers":["discount-15"]},
					{"productId":"product-4","quantity":5,"modifiers":["tax-25"]}
				]}
			]}}`,
		},
		{
			name:  "federation entity lookup (CallKindEntity)",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Product { id name } ...on Storage { id name } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Product","id":"1"},
				{"__typename":"Storage","id":"3"},
				{"__typename":"Product","id":"2"}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{TypeName: "Product", SelectionSet: "id"},
				{TypeName: "Storage", SelectionSet: "id"},
			},
		},
		{
			name:  "federation @requires (CallKindRequired)",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id name stockHealthScore } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","itemCount":100,"restockData":{"lastRestockDate":"2021-01-01"}},
				{"__typename":"Storage","id":"2","itemCount":200,"restockData":{"lastRestockDate":"2021-01-02"}}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{TypeName: "Storage", SelectionSet: "id"},
				{TypeName: "Storage", FieldName: "stockHealthScore", SelectionSet: "itemCount restockData { lastRestockDate }"},
			},
		},
		{
			name:  "federation @requires + field resolver (CallKindResolve)",
			query: `query($representations: [_Any!]!, $checkHealth: Boolean!) { _entities(representations: $representations) { ...on Storage { __typename id tagSummary storageStatus(checkHealth: $checkHealth) { ... on ActionSuccess { message } ... on ActionError { message code } } } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","tags":["electronics","gadgets","sale"]},
				{"__typename":"Storage","id":"2","tags":["books","fiction"]}
			],"checkHealth":false}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{TypeName: "Storage", SelectionSet: "id"},
				{TypeName: "Storage", FieldName: "tagSummary", SelectionSet: "tags"},
			},
		},
	}

	schemaDoc := grpctest.MustGraphQLSchema(t)
	protoSchema := grpctest.MustProtoSchema(t)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			queryDoc, report := astparser.ParseGraphqlDocumentString(tc.query)
			require.False(t, report.HasErrors(), "failed to parse query: %s", report.Error())

			compiler, err := NewProtoCompiler(protoSchema, testMapping())
			require.NoError(t, err)

			ds, err := NewDataSource(NewGRPCTransport(conn), DataSourceConfig{
				Operation:         &queryDoc,
				Definition:        &schemaDoc,
				SubgraphName:      "Products",
				Mapping:           testMapping(),
				Compiler:          compiler,
				FederationConfigs: tc.federationConfigs,
			})
			require.NoError(t, err)

			input := fmt.Appendf(nil, `{"query":%q,"body":%s}`, tc.query, tc.vars)

			grpcOut, grpcErr := ds.loadWithGRPC(context.Background(), input)
			connectOut, connectErr := ds.loadWithConnect(context.Background(), nil, input)

			require.NoError(t, grpcErr)
			require.NoError(t, connectErr)
			require.JSONEq(t, string(grpcOut), string(connectOut),
				"loadWithConnect output must match loadWithGRPC output")
		})
	}
}
