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
		optionalTags: [String!]
		metadata: StorageMetadata!
		storageKind: CategoryKind!
		primaryItem: StorageItem!
		lastStorageOperation: StorageOperationResult!
		securitySetup: SecuritySetup!
		stockHealthScore: Float!
		tagSummary: String!
		metadataScore: Float!
		filteredTagSummary(prefix: String!): String
		itemInfo: String!
		operationReport: String!
		securitySummary: String!
		itemHandlerInfo: String!
		itemSpecsInfo: String!
		deepItemInfo: String!
		recommendedItem: StorageItem!
		recommendedItems: [StorageItem!]!
		latestOperation: StorageOperationResult!
		optionalLatestOperation: StorageOperationResult
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
` + compositeTypeDefinitions

// compositeTypeDefinitions holds the abstract types (interface/union) and their members used by the
// composite-type @requires cases. They are shared verbatim by the supergraph and the owning subgraph
// SDL: as value types both subgraphs know them, the owning subgraph produces them as @requires input
// and the gRPC subgraph returns them from @requires fields.
const compositeTypeDefinitions = `
	enum CategoryKind {
		BOOK
		ELECTRONICS
		FURNITURE
		OTHER
	}

	interface StorageItem {
		id: ID!
		name: String!
		weight: Float!
	}

	type PalletItem implements StorageItem {
		id: ID!
		name: String!
		weight: Float!
		palletCount: Int!
		handler: ItemHandler!
		specs: PalletSpecs!
	}

	type ContainerItem implements StorageItem {
		id: ID!
		name: String!
		weight: Float!
		containerSize: String!
		handler: ItemHandler!
		specs: ContainerSpecs!
	}

	type ItemHandler {
		id: ID!
		name: String!
		assignedItem: StorageItem!
	}

	type PalletSpecs {
		name: String!
		maxWeight: Float!
		dimensions: Dimensions!
	}

	type ContainerSpecs {
		name: String!
		volume: Float!
		dimensions: Dimensions!
	}

	type Dimensions {
		length: Float!
		width: Float!
		height: Float!
	}

	union StorageOperationResult = StorageSuccess | StorageFailure

	type StorageSuccess {
		message: String!
		completedAt: String!
	}

	type StorageFailure {
		message: String!
		errorCode: String!
	}

	type SecuritySetup {
		securityLevel: String!
		primaryItem: StorageItem!
	}
`

// owningSubgraphSDL is the single, shared SDL for the "owning" subgraph across all @requires cases.
// It owns the entity root fields plus every field the gRPC subgraph consumes via @requires (from the
// gRPC perspective those are @external). Individual cases only vary the mocked response, never this.
const owningSubgraphSDL = `
	type Query {
		storageProvider(id: ID!): Storage
		warehouseProvider(id: ID!): Warehouse
	}

	type Storage @key(fields: "id") {
		id: ID!
		itemCount: Int!
		restockData: RestockData!
		tags: [String!]!
		optionalTags: [String!]
		metadata: StorageMetadata!
		storageKind: CategoryKind!
		primaryItem: StorageItem!
		lastStorageOperation: StorageOperationResult!
		securitySetup: SecuritySetup!
	}

	type Warehouse @key(fields: "id") {
		id: ID!
		inventoryCount: Int!
		restockData: RestockData!
	}

	type RestockData {
		lastRestockDate: String!
	}

	type StorageMetadata {
		capacity: Int!
		zone: String!
	}
` + compositeTypeDefinitions

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

// newOwningSubgraphMetadata returns a fresh metadata instance describing what the owning subgraph
// owns (a superset covering every @requires input across the cases) plus the entity @keys. A fresh
// instance is returned per call because NewDataSourceConfiguration mutates it via Init(), and the
// subtests run in parallel.
func newOwningSubgraphMetadata() *plan.DataSourceMetadata {
	return &plan.DataSourceMetadata{
		RootNodes: []plan.TypeField{
			{TypeName: "Query", FieldNames: []string{"storageProvider", "warehouseProvider"}},
			{TypeName: "Storage", FieldNames: []string{"id", "itemCount", "restockData", "tags", "optionalTags", "metadata", "storageKind", "primaryItem", "lastStorageOperation", "securitySetup"}},
			{TypeName: "Warehouse", FieldNames: []string{"id", "inventoryCount", "restockData"}},
		},
		ChildNodes: []plan.TypeField{
			{TypeName: "RestockData", FieldNames: []string{"lastRestockDate"}},
			{TypeName: "StorageMetadata", FieldNames: []string{"capacity", "zone"}},
			// The abstract types and their members are value types owned by both subgraphs. The owning
			// subgraph produces them as @requires input for the gRPC subgraph.
			{TypeName: "StorageItem", FieldNames: []string{"id", "name", "weight"}},
			{TypeName: "PalletItem", FieldNames: []string{"id", "name", "weight", "palletCount", "handler", "specs"}},
			{TypeName: "ContainerItem", FieldNames: []string{"id", "name", "weight", "containerSize", "handler", "specs"}},
			{TypeName: "ItemHandler", FieldNames: []string{"id", "name", "assignedItem"}},
			{TypeName: "PalletSpecs", FieldNames: []string{"name", "maxWeight", "dimensions"}},
			{TypeName: "ContainerSpecs", FieldNames: []string{"name", "volume", "dimensions"}},
			{TypeName: "Dimensions", FieldNames: []string{"length", "width", "height"}},
			{TypeName: "StorageSuccess", FieldNames: []string{"message", "completedAt"}},
			{TypeName: "StorageFailure", FieldNames: []string{"message", "errorCode"}},
			{TypeName: "SecuritySetup", FieldNames: []string{"securityLevel", "primaryItem"}},
		},
		FederationMetaData: plan.FederationMetaData{
			Keys: plan.FederationFieldConfigurations{
				{TypeName: "Storage", SelectionSet: "id"},
				{TypeName: "Warehouse", SelectionSet: "id"},
			},
		},
	}
}

// requiresTestCase is one @requires scenario exercised end-to-end through the engine. Only the
// mocked owning-subgraph response, the operation and the assertion vary; the owning subgraph's SDL
// and metadata are shared across all cases.
type requiresTestCase struct {
	name string
	// owningResponseJSON is the fixed upstream response the owning subgraph returns; it must contain
	// the entity's __typename, key and the fields referenced by the @requires selection set so the
	// planner can build the representation for the jump.
	owningResponseJSON string
	operation          string
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
			name:               "Storage scalar @requires with nested selection",
			owningResponseJSON: `{"data":{"storageProvider":{"__typename":"Storage","id":"1","itemCount":100,"restockData":{"__typename":"RestockData","lastRestockDate":"2021-01-01"}}}}`,
			operation:          `query { storageProvider(id: "1") { name stockHealthScore } }`,
			assert:             expectJSON(`{"data":{"storageProvider":{"name":"Storage 1","stockHealthScore":20}}}`),
		},
		{
			// @requires on a list scalar: tagSummary requires "tags". Mock joins tags with ", ".
			name:               "Storage @requires a scalar list",
			owningResponseJSON: `{"data":{"storageProvider":{"__typename":"Storage","id":"1","tags":["alpha","beta","gamma"]}}}`,
			operation:          `query { storageProvider(id: "1") { tagSummary } }`,
			assert:             expectJSON(`{"data":{"storageProvider":{"tagSummary":"alpha, beta, gamma"}}}`),
		},
		{
			// @requires on nested object fields: metadataScore requires "metadata { capacity zone }".
			// Mock: capacity * zoneWeight; zone "A" => 1.0, so 100 * 1.0 = 100.0.
			name:               "Storage @requires nested object fields",
			owningResponseJSON: `{"data":{"storageProvider":{"__typename":"Storage","id":"1","metadata":{"capacity":100,"zone":"A"}}}}`,
			operation:          `query { storageProvider(id: "1") { metadataScore } }`,
			assert:             expectJSON(`{"data":{"storageProvider":{"metadataScore":100}}}`),
		},
		{
			// Same @requires machinery on a different entity (Warehouse.stockHealthScore requires
			// "inventoryCount restockData { lastRestockDate }"), which exercises the error path: the
			// LookupWarehouseById mock deliberately returns one fewer entity than requested (see
			// grpctest/mockservice_lookup.go), so the engine must surface the subgraph entity-count
			// error and null the field rather than fabricate data. This still verifies Warehouse's
			// @requires config is wired and that the jump is planned for a second entity type.
			name:               "Warehouse @requires surfaces subgraph entity-count error",
			owningResponseJSON: `{"data":{"warehouseProvider":{"__typename":"Warehouse","id":"2","inventoryCount":200,"restockData":{"__typename":"RestockData","lastRestockDate":"2021-01-02"}}}}`,
			operation:          `query { warehouseProvider(id: "2") { stockHealthScore } }`,
			assert: func(t *testing.T, response string) {
				require.Contains(t, response, "entity type Warehouse received 0 entities", "response was: %s", response)
				require.Contains(t, response, `"warehouseProvider":null`, "response was: %s", response)
			},
		},
		{
			// @requires combined with a field argument: filteredTagSummary(prefix) requires "tags".
			// Mock keeps tags with the given prefix: prefix "ap" over [apple apricot banana] => "apple, apricot".
			name:               "Storage @requires with a field argument",
			owningResponseJSON: `{"data":{"storageProvider":{"__typename":"Storage","id":"1","tags":["apple","apricot","banana"]}}}`,
			operation:          `query { storageProvider(id: "1") { filteredTagSummary(prefix: "ap") } }`,
			assert:             expectJSON(`{"data":{"storageProvider":{"filteredTagSummary":"apple, apricot"}}}`),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Both subgraph setups live side by side: the owning subgraph provides the entity key and
			// the @requires inputs, the gRPC subgraph resolves the @requires field.
			owningDS := setupOwningSubgraph(t, tc.owningResponseJSON)
			grpcDS := setupGRPCProductsSubgraph(t, conn)

			response := runRequiresOperation(t, []plan.DataSource{owningDS, grpcDS}, tc.operation)

			tc.assert(t, response)
		})
	}
}

// TestGRPCSubgraphRequiresCompositeTypesFullExecution exercises @requires against composite
// (abstract) types end-to-end: the owning subgraph is asked for a representation containing an
// interface/union, the representation travels to the gRPC subgraph, and the gRPC subgraph resolves
// the @requires field. Two directions are covered:
//
//   - abstract types inside the @requires selection set (itemInfo, operationReport,
//     securitySummary, itemHandlerInfo, itemSpecsInfo, deepItemInfo) — here the concrete member has
//     to be written into the request's protobuf oneof.
//   - abstract types as the @requires field's return type (recommendedItem, recommendedItems,
//     latestOperation, optionalLatestOperation) — here the concrete member has to be read back out
//     of the response's oneof, so that __typename reports the concrete type and only the matching
//     inline fragment's fields are returned.
func TestGRPCSubgraphRequiresCompositeTypesFullExecution(t *testing.T) {
	t.Parallel()

	conn := setupGRPCTestGoPluginServer(t)

	testCases := []requiresTestCase{
		{
			// Pattern 1: flat interface in the @requires selection set. The owning subgraph resolves
			// primaryItem to a PalletItem, so the mock must see the PalletItem oneof member.
			name:               "Storage @requires an interface with inline fragments",
			owningResponseJSON: `{"data":{"storageProvider":{"__typename":"Storage","id":"1","primaryItem":{"__typename":"PalletItem","name":"Euro pallet","palletCount":12}}}}`,
			operation:          `query { storageProvider(id: "1") { itemInfo } }`,
			assert:             expectJSON(`{"data":{"storageProvider":{"itemInfo":"Pallet: Euro pallet (count: 12)"}}}`),
		},
		{
			// Pattern 2: flat union in the @requires selection set, resolved to the failure member.
			name:               "Storage @requires a union with inline fragments",
			owningResponseJSON: `{"data":{"storageProvider":{"__typename":"Storage","id":"1","lastStorageOperation":{"__typename":"StorageFailure","message":"Disk full","errorCode":"E_DISK"}}}}`,
			operation:          `query { storageProvider(id: "1") { operationReport } }`,
			assert:             expectJSON(`{"data":{"storageProvider":{"operationReport":"Failure: Disk full (code: E_DISK)"}}}`),
		},
		{
			// Pattern 3: concrete type (SecuritySetup) wrapping an abstract type.
			name:               "Storage @requires a concrete type wrapping an interface",
			owningResponseJSON: `{"data":{"storageProvider":{"__typename":"Storage","id":"1","securitySetup":{"__typename":"SecuritySetup","securityLevel":"HIGH","primaryItem":{"__typename":"ContainerItem","name":"Reefer","containerSize":"40ft"}}}}}`,
			operation:          `query { storageProvider(id: "1") { securitySummary } }`,
			assert:             expectJSON(`{"data":{"storageProvider":{"securitySummary":"[HIGH] Container: Reefer (size: 40ft)"}}}`),
		},
		{
			// Pattern 4: concrete message (handler) selected inside an inline fragment.
			name:               "Storage @requires a concrete type inside an inline fragment",
			owningResponseJSON: `{"data":{"storageProvider":{"__typename":"Storage","id":"1","primaryItem":{"__typename":"ContainerItem","handler":{"__typename":"ItemHandler","name":"Dock crew"}}}}}`,
			operation:          `query { storageProvider(id: "1") { itemHandlerInfo } }`,
			assert:             expectJSON(`{"data":{"storageProvider":{"itemHandlerInfo":"ContainerHandler: Dock crew"}}}`),
		},
		{
			// Pattern 5: deep concrete nesting (specs → dimensions) inside an inline fragment.
			name:               "Storage @requires deeply nested concrete types inside an inline fragment",
			owningResponseJSON: `{"data":{"storageProvider":{"__typename":"Storage","id":"1","primaryItem":{"__typename":"PalletItem","specs":{"__typename":"PalletSpecs","name":"EUR-1","dimensions":{"__typename":"Dimensions","length":120,"width":80}}}}}}`,
			operation:          `query { storageProvider(id: "1") { itemSpecsInfo } }`,
			assert:             expectJSON(`{"data":{"storageProvider":{"itemSpecsInfo":"PalletSpecs: EUR-1 (120.0x80.0)"}}}`),
		},
		{
			// Pattern 6: a second abstract type nested behind a concrete intermediary
			// (primaryItem → handler → assignedItem), so two oneofs have to be written per request.
			name:               "Storage @requires a nested interface behind a concrete intermediary",
			owningResponseJSON: `{"data":{"storageProvider":{"__typename":"Storage","id":"1","primaryItem":{"__typename":"PalletItem","handler":{"__typename":"ItemHandler","assignedItem":{"__typename":"ContainerItem","name":"Nested reefer","containerSize":"20ft"}}}}}}`,
			operation:          `query { storageProvider(id: "1") { deepItemInfo } }`,
			assert:             expectJSON(`{"data":{"storageProvider":{"deepItemInfo":"PalletHandler->Container: Nested reefer (size: 20ft)"}}}`),
		},
		{
			// Pattern 7: the @requires field returns an interface. capacity 200 > 100 => PalletItem,
			// palletCount = capacity / 10 = 20, weight = palletCount * 12.5 = 250. Interface fields are
			// selected directly next to the inline fragments.
			name:               "Storage @requires field returning an interface",
			owningResponseJSON: `{"data":{"storageProvider":{"__typename":"Storage","id":"1","metadata":{"__typename":"StorageMetadata","capacity":200,"zone":"A"}}}}`,
			operation:          `query { storageProvider(id: "1") { recommendedItem { __typename id name weight ... on PalletItem { palletCount } ... on ContainerItem { containerSize } } } }`,
			assert:             expectJSON(`{"data":{"storageProvider":{"recommendedItem":{"__typename":"PalletItem","id":"pallet-A-200","name":"Pallet for zone A","weight":250,"palletCount":20}}}}`),
		},
		{
			// Pattern 7, other member and one level deeper: capacity 50 <= 100 => ContainerItem, whose
			// handler.assignedItem is itself abstract and must resolve to its own concrete member.
			name:               "Storage @requires field returning an interface with a nested interface",
			owningResponseJSON: `{"data":{"storageProvider":{"__typename":"Storage","id":"2","metadata":{"__typename":"StorageMetadata","capacity":50,"zone":"B"}}}}`,
			operation:          `query { storageProvider(id: "2") { recommendedItem { __typename ... on PalletItem { name palletCount } ... on ContainerItem { name containerSize handler { name assignedItem { __typename ... on PalletItem { name palletCount } ... on ContainerItem { name containerSize } } } } } } }`,
			assert:             expectJSON(`{"data":{"storageProvider":{"recommendedItem":{"__typename":"ContainerItem","name":"Container for zone B","containerSize":"50L","handler":{"name":"Handler for Container for zone B","assignedItem":{"__typename":"PalletItem","name":"Container for zone B assigned pallet","palletCount":7}}}}}}`),
		},
		{
			// Pattern 8: the @requires field returns a list of an interface — one entry per required
			// tag, alternating between both concrete members by index.
			name:               "Storage @requires field returning a list of an interface",
			owningResponseJSON: `{"data":{"storageProvider":{"__typename":"Storage","id":"1","tags":["alpha","beta","gamma"]}}}`,
			operation:          `query { storageProvider(id: "1") { recommendedItems { __typename name ... on PalletItem { palletCount } ... on ContainerItem { containerSize } } } }`,
			assert: expectJSON(`{"data":{"storageProvider":{"recommendedItems":[
				{"__typename":"PalletItem","name":"Pallet alpha","palletCount":1},
				{"__typename":"ContainerItem","name":"Container beta","containerSize":"BETA"},
				{"__typename":"PalletItem","name":"Pallet gamma","palletCount":3}
			]}}}`),
		},
		{
			// Pattern 8 with an empty required list: an empty list, not null.
			name:               "Storage @requires field returning an empty list of an interface",
			owningResponseJSON: `{"data":{"storageProvider":{"__typename":"Storage","id":"1","tags":[]}}}`,
			operation:          `query { storageProvider(id: "1") { recommendedItems { __typename name } } }`,
			assert:             expectJSON(`{"data":{"storageProvider":{"recommendedItems":[]}}}`),
		},
		{
			// Pattern 9: the @requires field returns a union, keyed off the required enum. A known kind
			// yields StorageSuccess, so the StorageFailure fragment must contribute nothing.
			name:               "Storage @requires field returning a union",
			owningResponseJSON: `{"data":{"storageProvider":{"__typename":"Storage","id":"1","storageKind":"ELECTRONICS"}}}`,
			operation:          `query { storageProvider(id: "1") { latestOperation { __typename ... on StorageSuccess { message completedAt } ... on StorageFailure { message errorCode } } } }`,
			assert:             expectJSON(`{"data":{"storageProvider":{"latestOperation":{"__typename":"StorageSuccess","message":"Operation completed for CATEGORY_KIND_ELECTRONICS","completedAt":"2024-01-01T00:00:00Z"}}}}`),
		},
		{
			// Pattern 9, the other union member: an unknown kind yields StorageFailure.
			name:               "Storage @requires field returning the other union member",
			owningResponseJSON: `{"data":{"storageProvider":{"__typename":"Storage","id":"1","storageKind":"OTHER"}}}`,
			operation:          `query { storageProvider(id: "1") { latestOperation { __typename ... on StorageSuccess { message completedAt } ... on StorageFailure { message errorCode } } } }`,
			assert:             expectJSON(`{"data":{"storageProvider":{"latestOperation":{"__typename":"StorageFailure","message":"Operation failed for CATEGORY_KIND_OTHER","errorCode":"UNSUPPORTED_KIND"}}}}`),
		},
		{
			// Pattern 10: nullable union return type. An odd number of required optional tags yields
			// StorageSuccess.
			name:               "Storage @requires field returning a nullable union",
			owningResponseJSON: `{"data":{"storageProvider":{"__typename":"Storage","id":"1","optionalTags":["alpha"]}}}`,
			operation:          `query { storageProvider(id: "1") { optionalLatestOperation { __typename ... on StorageSuccess { message completedAt } ... on StorageFailure { errorCode } } } }`,
			assert:             expectJSON(`{"data":{"storageProvider":{"optionalLatestOperation":{"__typename":"StorageSuccess","message":"Operation completed for tags: alpha","completedAt":"2024-01-02T00:00:00Z"}}}}`),
		},
		{
			// Pattern 10 with no required tags: the mock returns no union value at all, which must
			// surface as null rather than an empty object or an error.
			name:               "Storage @requires field returning a nullable union resolves to null",
			owningResponseJSON: `{"data":{"storageProvider":{"__typename":"Storage","id":"1","optionalTags":null}}}`,
			operation:          `query { storageProvider(id: "1") { optionalLatestOperation { __typename ... on StorageSuccess { message } ... on StorageFailure { errorCode } } } }`,
			assert:             expectJSON(`{"data":{"storageProvider":{"optionalLatestOperation":null}}}`),
		},
		{
			// Abstract return types alongside a scalar @requires field and the entity lookup, so all
			// three kinds of gRPC call are planned against the same representation.
			name:               "Storage @requires abstract return types combined with a scalar field and a lookup",
			owningResponseJSON: `{"data":{"storageProvider":{"__typename":"Storage","id":"1","tags":["alpha","beta"],"storageKind":"BOOK"}}}`,
			operation:          `query { storageProvider(id: "1") { name tagSummary recommendedItems { __typename name } latestOperation { __typename ... on StorageSuccess { message } ... on StorageFailure { errorCode } } } }`,
			assert: expectJSON(`{"data":{"storageProvider":{
				"name":"Storage 1",
				"tagSummary":"alpha, beta",
				"recommendedItems":[
					{"__typename":"PalletItem","name":"Pallet alpha"},
					{"__typename":"ContainerItem","name":"Container beta"}
				],
				"latestOperation":{"__typename":"StorageSuccess","message":"Operation completed for CATEGORY_KIND_BOOK"}
			}}}`),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			owningDS := setupOwningSubgraph(t, tc.owningResponseJSON)
			grpcDS := setupGRPCProductsSubgraph(t, conn)

			response := runRequiresOperation(t, []plan.DataSource{owningDS, grpcDS}, tc.operation)

			tc.assert(t, response)
		})
	}
}

// setupOwningSubgraph builds the "owning" subgraph: a graphql_datasource over an httptest.Server
// that returns responseJSON for any request. Its SDL (owningSubgraphSDL) and metadata are shared
// across all cases; only responseJSON varies. It owns the entity root fields plus the fields the
// gRPC subgraph consumes via @requires.
func setupOwningSubgraph(t *testing.T, responseJSON string) plan.DataSource {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(responseJSON))
	}))
	t.Cleanup(server.Close)

	config, err := graphql_datasource.NewConfiguration(graphql_datasource.ConfigurationInput{
		Fetch: &graphql_datasource.FetchConfiguration{URL: server.URL},
		SchemaConfiguration: mustSchemaConfig(t,
			&graphql_datasource.FederationConfiguration{Enabled: true, ServiceSDL: owningSubgraphSDL},
			owningSubgraphSDL,
		),
	})
	require.NoError(t, err)

	ds, err := plan.NewDataSourceConfiguration[graphql_datasource.Configuration](
		"owning-subgraph",
		mustFactory(t, http.DefaultClient),
		newOwningSubgraphMetadata(),
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
