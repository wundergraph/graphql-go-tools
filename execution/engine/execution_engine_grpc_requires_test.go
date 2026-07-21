//go:build !windows

package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	grpcdatasource "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/grpc_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest/mapping"
)

// requiresSupergraphSDL is the shared supergraph (engine) schema for the @requires tests. It
// describes every field the test operations select. Federation ownership is NOT expressed here —
// it lives in each subgraph's ServiceSDL + DataSourceMetadata. Root fields returning the entities
// (storageProvider/warehouseProvider) are owned by the "owning" subgraph; the @requires fields and
// name/location are owned by the gRPC subgraph.
const requiresSupergraphSDL = `
	type Query {
		storageProvider(id: ID!): Storage
		warehouseProvider(id: ID!): Warehouse
	}

	type Storage {
		id: ID!
		name: String!
		location: String!
		itemCount: Int!
		restockData: RestockData!
		tags: [String!]!
		metadata: StorageMetadata!
		stockHealthScore: Float!
		tagSummary: String!
		metadataScore: Float!
		filteredTagSummary(prefix: String!): String
	}

	type Warehouse {
		id: ID!
		name: String!
		location: String!
		inventoryCount: Int!
		restockData: RestockData!
		stockHealthScore: Float!
	}

	type RestockData {
		lastRestockDate: String!
	}

	type StorageMetadata {
		capacity: Int!
		zone: String!
		priority: Int!
	}
`

// requiresFieldConfigurations covers the arguments of every field the test operations use: the
// entity root fields and the @requires field that also takes an argument.
var requiresFieldConfigurations = plan.FieldConfigurations{
	{
		TypeName:  "Query",
		FieldName: "storageProvider",
		Arguments: []plan.ArgumentConfiguration{{Name: "id", SourceType: plan.FieldArgumentSource}},
	},
	{
		TypeName:  "Query",
		FieldName: "warehouseProvider",
		Arguments: []plan.ArgumentConfiguration{{Name: "id", SourceType: plan.FieldArgumentSource}},
	},
	{
		TypeName:  "Storage",
		FieldName: "filteredTagSummary",
		Arguments: []plan.ArgumentConfiguration{{Name: "prefix", SourceType: plan.FieldArgumentSource}},
	},
}

// owningSubgraphConfig describes the "owning" subgraph for a single test case: the subgraph that
// exposes the entity via a root field and provides the fields the gRPC subgraph consumes through
// @requires (its @external fields). Its upstream returns responseJSON verbatim for any request.
type owningSubgraphConfig struct {
	// serviceSDL is the federation SDL for this subgraph (root field, @key, owned/external fields).
	serviceSDL string
	// responseJSON is the fixed upstream response; it must contain the entity's __typename, key and
	// the fields referenced by the @requires selection set so the planner can build the representation.
	responseJSON string
	// metadata declares which nodes this subgraph owns plus the entity @key.
	metadata *plan.DataSourceMetadata
}

// requiresTestCase is one @requires scenario exercised end-to-end through the engine.
type requiresTestCase struct {
	name      string
	owning    owningSubgraphConfig
	operation string
	// assert validates the raw engine response for this case.
	assert func(t *testing.T, response string)
}

// expectJSON asserts the engine response equals the given JSON (order-independent).
func expectJSON(expected string) func(t *testing.T, response string) {
	return func(t *testing.T, response string) {
		require.JSONEq(t, expected, response)
	}
}

func TestGRPCSubgraphRequiresFullExecution(t *testing.T) {
	t.Parallel()

	conn := setupGRPCTestGoPluginServer(t)

	testCases := []requiresTestCase{
		{
			// Scalar @requires with a nested selection: itemCount + restockData { lastRestockDate }.
			// Also selects name (resolved by the gRPC entity lookup) to cover lookup + requires together.
			// stockHealthScore = itemCount*0.1 + 10 (restockData provided) = 100*0.1 + 10 = 20.0.
			name: "Storage scalar @requires with nested selection",
			owning: owningSubgraphConfig{
				serviceSDL: `
					type Query { storageProvider(id: ID!): Storage }
					type Storage @key(fields: "id") {
						id: ID!
						itemCount: Int!
						restockData: RestockData!
					}
					type RestockData { lastRestockDate: String! }
				`,
				responseJSON: `{"data":{"storageProvider":{"__typename":"Storage","id":"1","itemCount":100,"restockData":{"__typename":"RestockData","lastRestockDate":"2021-01-01"}}}}`,
				metadata: &plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{TypeName: "Query", FieldNames: []string{"storageProvider"}},
						{TypeName: "Storage", FieldNames: []string{"id", "itemCount", "restockData"}},
					},
					ChildNodes: []plan.TypeField{
						{TypeName: "RestockData", FieldNames: []string{"lastRestockDate"}},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{{TypeName: "Storage", SelectionSet: "id"}},
					},
				},
			},
			operation: `query { storageProvider(id: "1") { name stockHealthScore } }`,
			assert:    expectJSON(`{"data":{"storageProvider":{"name":"Storage 1","stockHealthScore":20}}}`),
		},
		{
			// @requires on a list scalar: tagSummary requires "tags". Mock joins tags with ", ".
			name: "Storage @requires a scalar list",
			owning: owningSubgraphConfig{
				serviceSDL: `
					type Query { storageProvider(id: ID!): Storage }
					type Storage @key(fields: "id") {
						id: ID!
						tags: [String!]!
					}
				`,
				responseJSON: `{"data":{"storageProvider":{"__typename":"Storage","id":"1","tags":["alpha","beta","gamma"]}}}`,
				metadata: &plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{TypeName: "Query", FieldNames: []string{"storageProvider"}},
						{TypeName: "Storage", FieldNames: []string{"id", "tags"}},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{{TypeName: "Storage", SelectionSet: "id"}},
					},
				},
			},
			operation: `query { storageProvider(id: "1") { tagSummary } }`,
			assert:    expectJSON(`{"data":{"storageProvider":{"tagSummary":"alpha, beta, gamma"}}}`),
		},
		{
			// @requires on nested object fields: metadataScore requires "metadata { capacity zone }".
			// Mock: capacity * zoneWeight; zone "A" => 1.0, so 100 * 1.0 = 100.0.
			name: "Storage @requires nested object fields",
			owning: owningSubgraphConfig{
				serviceSDL: `
					type Query { storageProvider(id: ID!): Storage }
					type Storage @key(fields: "id") {
						id: ID!
						metadata: StorageMetadata!
					}
					type StorageMetadata { capacity: Int! zone: String! }
				`,
				responseJSON: `{"data":{"storageProvider":{"__typename":"Storage","id":"1","metadata":{"capacity":100,"zone":"A"}}}}`,
				metadata: &plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{TypeName: "Query", FieldNames: []string{"storageProvider"}},
						{TypeName: "Storage", FieldNames: []string{"id", "metadata"}},
					},
					ChildNodes: []plan.TypeField{
						{TypeName: "StorageMetadata", FieldNames: []string{"capacity", "zone"}},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{{TypeName: "Storage", SelectionSet: "id"}},
					},
				},
			},
			operation: `query { storageProvider(id: "1") { metadataScore } }`,
			assert:    expectJSON(`{"data":{"storageProvider":{"metadataScore":100}}}`),
		},
		{
			// Same @requires machinery on a different entity (Warehouse.stockHealthScore requires
			// "inventoryCount restockData { lastRestockDate }"), which exercises the error path: the
			// LookupWarehouseById mock deliberately returns one fewer entity than requested (see
			// grpctest/mockservice_lookup.go), so the engine must surface the subgraph entity-count
			// error and null the field rather than fabricate data. This still verifies Warehouse's
			// @requires config is wired and that the jump is planned for a second entity type.
			name: "Warehouse @requires surfaces subgraph entity-count error",
			owning: owningSubgraphConfig{
				serviceSDL: `
					type Query { warehouseProvider(id: ID!): Warehouse }
					type Warehouse @key(fields: "id") {
						id: ID!
						inventoryCount: Int!
						restockData: RestockData!
					}
					type RestockData { lastRestockDate: String! }
				`,
				responseJSON: `{"data":{"warehouseProvider":{"__typename":"Warehouse","id":"2","inventoryCount":200,"restockData":{"__typename":"RestockData","lastRestockDate":"2021-01-02"}}}}`,
				metadata: &plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{TypeName: "Query", FieldNames: []string{"warehouseProvider"}},
						{TypeName: "Warehouse", FieldNames: []string{"id", "inventoryCount", "restockData"}},
					},
					ChildNodes: []plan.TypeField{
						{TypeName: "RestockData", FieldNames: []string{"lastRestockDate"}},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{{TypeName: "Warehouse", SelectionSet: "id"}},
					},
				},
			},
			operation: `query { warehouseProvider(id: "2") { stockHealthScore } }`,
			assert: func(t *testing.T, response string) {
				require.Contains(t, response, "entity type Warehouse received 0 entities", "response was: %s", response)
				require.Contains(t, response, `"warehouseProvider":null`, "response was: %s", response)
			},
		},
		{
			// @requires combined with a field argument: filteredTagSummary(prefix) requires "tags".
			// Mock keeps tags with the given prefix: prefix "ap" over [apple apricot banana] => "apple, apricot".
			name: "Storage @requires with a field argument",
			owning: owningSubgraphConfig{
				serviceSDL: `
					type Query { storageProvider(id: ID!): Storage }
					type Storage @key(fields: "id") {
						id: ID!
						tags: [String!]!
					}
				`,
				responseJSON: `{"data":{"storageProvider":{"__typename":"Storage","id":"1","tags":["apple","apricot","banana"]}}}`,
				metadata: &plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{TypeName: "Query", FieldNames: []string{"storageProvider"}},
						{TypeName: "Storage", FieldNames: []string{"id", "tags"}},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{{TypeName: "Storage", SelectionSet: "id"}},
					},
				},
			},
			operation: `query { storageProvider(id: "1") { filteredTagSummary(prefix: "ap") } }`,
			assert:    expectJSON(`{"data":{"storageProvider":{"filteredTagSummary":"apple, apricot"}}}`),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Both subgraph setups live side by side: the owning subgraph provides the entity key and
			// the @requires inputs, the gRPC subgraph resolves the @requires field.
			owningDS := setupOwningSubgraph(t, tc.owning)
			grpcDS := setupGRPCProductsSubgraph(t, conn)

			response := runRequiresOperation(t, []plan.DataSource{owningDS, grpcDS}, tc.operation)

			tc.assert(t, response)
		})
	}
}

// setupOwningSubgraph builds an "owning" subgraph from cfg: a graphql_datasource over an
// httptest.Server that returns cfg.responseJSON for any request. It owns a root field returning the
// entity plus the fields the gRPC subgraph consumes via @requires.
func setupOwningSubgraph(t *testing.T, cfg owningSubgraphConfig) plan.DataSource {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(cfg.responseJSON))
	}))
	t.Cleanup(server.Close)

	config, err := graphql_datasource.NewConfiguration(graphql_datasource.ConfigurationInput{
		Fetch: &graphql_datasource.FetchConfiguration{URL: server.URL},
		SchemaConfiguration: mustSchemaConfig(t,
			&graphql_datasource.FederationConfiguration{Enabled: true, ServiceSDL: cfg.serviceSDL},
			cfg.serviceSDL,
		),
	})
	require.NoError(t, err)

	ds, err := plan.NewDataSourceConfiguration[graphql_datasource.Configuration](
		"owning-subgraph",
		mustFactory(t, http.DefaultClient),
		cfg.metadata,
		config,
	)
	require.NoError(t, err)

	return ds
}

// setupGRPCProductsSubgraph builds the gRPC subgraph over the go-plugin harness. It reuses the
// shared grpctest datasource metadata (the full products metadata, incl. every entity's @key and
// @requires config) and mapping; fields/types absent from a given test's operation are simply never
// planned, so advertising the extra nodes is harmless. Its SchemaConfiguration uses the products SDL
// (with @key/@external/@requires) so the proto compiler maps operations correctly. This subgraph
// owns name/location and resolves the @requires fields; the entity keys' external inputs are owned
// by the owning subgraph.
func setupGRPCProductsSubgraph(t *testing.T, conn grpc.ClientConnInterface) plan.DataSource {
	t.Helper()

	grpcMapping := mapping.MustDefaultGRPCMapping(t)

	factory, err := graphql_datasource.NewFactoryGRPC(context.Background(), conn)
	require.NoError(t, err)

	protoSchema, err := grpctest.ProtoSchema()
	require.NoError(t, err)

	compiler, err := grpcdatasource.NewProtoCompiler(protoSchema, grpcMapping)
	require.NoError(t, err)

	grpcSchemaDoc, err := grpctest.GraphQLSchemaWithoutBaseDefinitions()
	require.NoError(t, err)
	subgraphSDL := string(grpcSchemaDoc.Input.RawBytes)

	config, err := graphql_datasource.NewConfiguration(graphql_datasource.ConfigurationInput{
		GRPC: &grpcdatasource.GRPCConfiguration{Mapping: grpcMapping, Compiler: compiler},
		SchemaConfiguration: mustSchemaConfig(t,
			&graphql_datasource.FederationConfiguration{Enabled: true, ServiceSDL: subgraphSDL},
			subgraphSDL,
		),
	})
	require.NoError(t, err)

	ds, err := plan.NewDataSourceConfiguration[graphql_datasource.Configuration](
		"grpc-subgraph",
		factory,
		grpctest.GetDataSourceMetadata(),
		config,
	)
	require.NoError(t, err)

	return ds
}

// runRequiresOperation builds an engine over the given data sources and the shared supergraph
// schema, executes the operation and returns the raw JSON response.
func runRequiresOperation(t *testing.T, dataSources []plan.DataSource, operation string) string {
	t.Helper()

	inputSchema, err := graphql.NewSchemaFromString(requiresSupergraphSDL)
	require.NoError(t, err)

	engineConf := NewConfiguration(inputSchema)
	engineConf.SetDataSources(dataSources)
	engineConf.SetFieldConfigurations(requiresFieldConfigurations)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine, err := NewExecutionEngine(ctx, abstractlogger.Noop{}, engineConf, resolve.ResolverOptions{
		MaxConcurrency:               1024,
		PropagateSubgraphErrors:      true,
		SubgraphErrorPropagationMode: resolve.SubgraphErrorPropagationModeWrapped,
	})
	require.NoError(t, err)

	request := graphql.Request{Query: operation}

	resultWriter := graphql.NewEngineResultWriter()
	require.NoError(t, engine.Execute(ctx, &request, &resultWriter))

	return resultWriter.String()
}
