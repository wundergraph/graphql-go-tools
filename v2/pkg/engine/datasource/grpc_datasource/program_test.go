package grpcdatasource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
)

// compileProgramFromQuery is a test helper that runs the full pipeline:
// parse schema -> parse query -> plan operation -> compile proto -> create runtime -> compile program
func compileProgramFromQuery(t *testing.T, operation string) *program {
	t.Helper()

	return compileProgramFromQueryWithFederation(t, operation, nil)
}

func compileProgramFromQueryWithFederation(t *testing.T, operation string, federationConfigs plan.FederationFieldConfigurations) *program {
	t.Helper()

	schemaDoc := grpctest.MustGraphQLSchema(t)
	queryDoc, report := astparser.ParseGraphqlDocumentString(operation)
	if report.HasErrors() {
		t.Fatalf("failed to parse query: %s", report.Error())
	}

	mapping := testMapping()

	var planner PlanVisitor
	var err error
	if len(federationConfigs) > 0 {
		planner, err = NewPlanner("Products", mapping, federationConfigs)
		require.NoError(t, err)
	} else {
		planner = newRPCPlanVisitor(rpcPlanVisitorConfig{
			subgraphName: "Products",
			mapping:      mapping,
		})
	}

	executionPlan, err := planner.PlanOperation(&queryDoc, &schemaDoc)
	require.NoError(t, err)

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), mapping)
	require.NoError(t, err)

	runtime, err := newSchemaRuntime(compiler.doc)
	require.NoError(t, err)

	p, err := compileProgram(executionPlan, runtime)
	require.NoError(t, err)

	return p
}

func TestCompileProgram_SimpleQuery(t *testing.T) {
	t.Parallel()

	p := compileProgramFromQuery(t, `query { users { id name } }`)

	require.Len(t, p.stages, 1, "expected 1 stage for simple query")
	require.Len(t, p.stages[0].fetches, 1, "expected 1 fetch in stage 0")

	fetch := p.stages[0].fetches[0]
	assert.Equal(t, CallKindStandard, fetch.kind)
	assert.Equal(t, "productv1.ProductService", fetch.serviceName)
	assert.Equal(t, "QueryUsers", fetch.methodName)
	assert.Equal(t, "/productv1.ProductService/QueryUsers", fetch.methodFullName)
	assert.Nil(t, fetch.dependentCall)

	// Request
	require.NotNil(t, fetch.request)
	require.NotNil(t, fetch.request.message)
	assert.Equal(t, "QueryUsersRequest", fetch.request.message.name)
	assert.Nil(t, fetch.request.context, "standard call should have no context")

	// Wire message compiled
	require.NotNil(t, fetch.request.wire, "wire message should be compiled")

	// Response
	require.NotNil(t, fetch.response)
	assert.Equal(t, "QueryUsersResponse", fetch.response.responseType.name)
}

func TestCompileProgram_QueryWithArguments(t *testing.T) {
	t.Parallel()

	p := compileProgramFromQuery(t, `query GetUser { user(id: "1") { id name } }`)

	require.Len(t, p.stages, 1)
	require.Len(t, p.stages[0].fetches, 1)

	fetch := p.stages[0].fetches[0]
	assert.Equal(t, "QueryUser", fetch.methodName)
	assert.Equal(t, "QueryUserRequest", fetch.request.message.name)

	// Request should have an id field
	require.NotEmpty(t, fetch.request.fields)

	idField := fetch.request.fields[0]
	assert.Equal(t, "id", idField.jsonPath)
	assert.Equal(t, DataTypeString, idField.dataType)
}

func TestCompileProgram_MultipleRootFields(t *testing.T) {
	t.Parallel()

	p := compileProgramFromQuery(t, `query { users { id name } user(id: "1") { id name } }`)

	// Both calls have no dependencies, so they should be in stage 0
	require.Len(t, p.stages, 1)
	require.Len(t, p.stages[0].fetches, 2, "expected 2 parallel fetches in stage 0")

	methods := []string{p.stages[0].fetches[0].methodName, p.stages[0].fetches[1].methodName}
	assert.Contains(t, methods, "QueryUsers")
	assert.Contains(t, methods, "QueryUser")
}

func TestCompileProgram_FieldResolver(t *testing.T) {
	t.Parallel()

	p := compileProgramFromQuery(t, `query { categories { id productCount name } }`)

	// Stage 0: QueryCategories, Stage 1: ResolveCategoryProductCount (depends on stage 0)
	require.Len(t, p.stages, 2, "expected 2 stages for field resolver query")

	// Stage 0: the base query
	require.Len(t, p.stages[0].fetches, 1)
	baseFetch := p.stages[0].fetches[0]
	assert.Equal(t, CallKindStandard, baseFetch.kind)
	assert.Equal(t, "QueryCategories", baseFetch.methodName)

	// Stage 1: the field resolver
	require.Len(t, p.stages[1].fetches, 1)
	resolverFetch := p.stages[1].fetches[0]
	assert.Equal(t, CallKindResolve, resolverFetch.kind)
	assert.Equal(t, "ResolveCategoryProductCount", resolverFetch.methodName)
	assert.Equal(t, "/productv1.ProductService/ResolveCategoryProductCount", resolverFetch.methodFullName)

	// Resolver should have a dependent call pointing to the base query
	require.NotNil(t, resolverFetch.dependentCall)
	assert.Equal(t, "QueryCategories", resolverFetch.dependentCall.MethodName)

	// Resolver request should have context (for context-based fetches)
	require.NotNil(t, resolverFetch.request)
	require.NotNil(t, resolverFetch.request.context, "resolve call should have context")
	require.NotEmpty(t, resolverFetch.request.context.fields, "context should have fields")

	// Response
	assert.Equal(t, "ResolveCategoryProductCountResponse", resolverFetch.response.responseType.name)
}

func TestCompileProgram_FieldResolverWithArguments(t *testing.T) {
	t.Parallel()

	p := compileProgramFromQuery(t, `query($whoop: ProductCountFilter) { categories { id productCount(filters: $whoop) } }`)

	require.Len(t, p.stages, 2)

	resolverFetch := p.stages[1].fetches[0]
	assert.Equal(t, CallKindResolve, resolverFetch.kind)
	assert.Equal(t, "ResolveCategoryProductCount", resolverFetch.methodName)

	// Request should have both context and field_args fields
	require.NotNil(t, resolverFetch.request)
	require.NotNil(t, resolverFetch.request.message)

	// Check that the request message has fields (context + field_args)
	hasFieldArgs := false
	for _, f := range resolverFetch.request.fields {
		if f.jsonPath == "" && f.child != nil && f.child.name == "ResolveCategoryProductCountArgs" {
			hasFieldArgs = true
			break
		}
	}
	assert.True(t, hasFieldArgs, "resolver request should include field_args message")
}

func TestCompileProgram_EntityLookup(t *testing.T) {
	t.Parallel()

	p := compileProgramFromQueryWithFederation(t,
		`query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on Warehouse { __typename name location } } }`,
		plan.FederationFieldConfigurations{
			{
				TypeName:     "Warehouse",
				SelectionSet: "id",
			},
		},
	)

	require.Len(t, p.stages, 1, "entity lookup should be 1 stage")
	require.Len(t, p.stages[0].fetches, 1)

	fetch := p.stages[0].fetches[0]
	assert.Equal(t, CallKindEntity, fetch.kind)
	assert.Equal(t, "LookupWarehouseById", fetch.methodName)
	assert.Equal(t, "/productv1.ProductService/LookupWarehouseById", fetch.methodFullName)
	assert.Equal(t, "Warehouse", fetch.requestedEntityType)

	// Request
	require.NotNil(t, fetch.request)
	assert.Equal(t, "LookupWarehouseByIdRequest", fetch.request.message.name)
	require.NotNil(t, fetch.request.wire)

	// Response
	assert.Equal(t, "LookupWarehouseByIdResponse", fetch.response.responseType.name)
}

func TestCompileProgram_RequiredFields(t *testing.T) {
	t.Parallel()

	p := compileProgramFromQueryWithFederation(t,
		`query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on Warehouse { __typename name location stockHealthScore } } }`,
		plan.FederationFieldConfigurations{
			{
				TypeName:     "Warehouse",
				SelectionSet: "id",
			},
			{
				TypeName:     "Warehouse",
				FieldName:    "stockHealthScore",
				SelectionSet: "inventoryCount restockData { lastRestockDate }",
			},
		},
	)

	// Required calls don't use DependentCalls in the execution plan,
	// so both entity lookup and required field end up in the same stage.
	require.Len(t, p.stages, 1, "expected 1 stage for required fields")
	require.Len(t, p.stages[0].fetches, 2, "expected 2 fetches in stage 0")

	// Find entity and required fetches
	var entityFetch, requiredFetch *fetchProgram
	for i := range p.stages[0].fetches {
		f := &p.stages[0].fetches[i]
		switch f.kind {
		case CallKindEntity:
			entityFetch = f
		case CallKindRequired:
			requiredFetch = f
		}
	}

	require.NotNil(t, entityFetch, "should have entity fetch")
	assert.Equal(t, "LookupWarehouseById", entityFetch.methodName)

	require.NotNil(t, requiredFetch, "should have required fetch")
	assert.Equal(t, "RequireWarehouseStockHealthScoreById", requiredFetch.methodName)
	assert.Equal(t, "/productv1.ProductService/RequireWarehouseStockHealthScoreById", requiredFetch.methodFullName)

	// Request and wire should be compiled
	require.NotNil(t, requiredFetch.request)
	require.NotNil(t, requiredFetch.request.wire)
	require.NotNil(t, requiredFetch.request.message)
	assert.Equal(t, "RequireWarehouseStockHealthScoreByIdRequest", requiredFetch.request.message.name)

	// Verify request has context field with required data
	hasContextField := false
	for _, f := range requiredFetch.request.fields {
		if f.child != nil && f.child.name == "RequireWarehouseStockHealthScoreByIdContext" {
			hasContextField = true

			// Context should have key and fields sub-messages
			require.NotEmpty(t, f.child.fields, "context message should have fields")
			break
		}
	}
	assert.True(t, hasContextField, "required fetch request should have context field")

	// Response
	assert.Equal(t, "RequireWarehouseStockHealthScoreByIdResponse", requiredFetch.response.responseType.name)
}

func TestCompileProgram_NestedMessages(t *testing.T) {
	t.Parallel()

	p := compileProgramFromQuery(t, `query { nestedType { id name b { id name c { id name } } } }`)

	require.Len(t, p.stages, 1)
	require.Len(t, p.stages[0].fetches, 1)

	fetch := p.stages[0].fetches[0]
	assert.Equal(t, "QueryNestedType", fetch.methodName)
	assert.Equal(t, "QueryNestedTypeResponse", fetch.response.responseType.name)

	// Wire message should be compiled for nested structures
	require.NotNil(t, fetch.request.wire)
}

func TestCompileProgram_ResponsePath(t *testing.T) {
	t.Parallel()

	p := compileProgramFromQuery(t, `query { categories { id productCount } }`)

	require.Len(t, p.stages, 2)

	// The field resolver fetch should have a response path
	resolverFetch := p.stages[1].fetches[0]
	assert.Equal(t, CallKindResolve, resolverFetch.kind)
	require.NotEmpty(t, resolverFetch.responsePath, "field resolver should have a response path")
}

func TestCompileProgram_StageOrdering(t *testing.T) {
	t.Parallel()

	// A query with multiple field resolvers should properly order stages
	p := compileProgramFromQuery(t, `query { categories { id productCount popularityScore } }`)

	// Both field resolvers depend on stage 0, so they should both be in stage 1
	require.Len(t, p.stages, 2, "expected 2 stages")
	require.Len(t, p.stages[0].fetches, 1, "stage 0 should have 1 fetch (QueryCategories)")
	require.Len(t, p.stages[1].fetches, 2, "stage 1 should have 2 fetches (both field resolvers)")

	methods := []string{
		p.stages[1].fetches[0].methodName,
		p.stages[1].fetches[1].methodName,
	}
	assert.Contains(t, methods, "ResolveCategoryProductCount")
	assert.Contains(t, methods, "ResolveCategoryPopularityScore")
}

func TestCompileProgram_EnumField(t *testing.T) {
	t.Parallel()

	p := compileProgramFromQuery(t, `query { categories { id kind } }`)

	require.Len(t, p.stages, 1)
	fetch := p.stages[0].fetches[0]

	// Response message should reference the enum correctly
	assert.Equal(t, "QueryCategoriesResponse", fetch.response.responseType.name)
	assert.Equal(t, "QueryCategories", fetch.methodName)
}

func TestCompileProgram_WireMessageCompiled(t *testing.T) {
	t.Parallel()

	// Verify wire messages are compiled for various call kinds
	tests := []struct {
		name      string
		operation string
		fedConfig plan.FederationFieldConfigurations
	}{
		{
			name:      "standard query",
			operation: `query { users { id name } }`,
		},
		{
			name:      "query with arguments",
			operation: `query GetUser { user(id: "1") { id name } }`,
		},
		{
			name:      "entity lookup",
			operation: `query EntityLookup($representations: [_Any!]!) { _entities(representations: $representations) { ... on Warehouse { __typename name } } }`,
			fedConfig: plan.FederationFieldConfigurations{
				{TypeName: "Warehouse", SelectionSet: "id"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := compileProgramFromQueryWithFederation(t, tt.operation, tt.fedConfig)

			for i, stage := range p.stages {
				for j, fetch := range stage.fetches {
					require.NotNil(t, fetch.request, "stage[%d].fetch[%d] request should not be nil", i, j)
					require.NotNil(t, fetch.request.wire, "stage[%d].fetch[%d] wire message should not be nil", i, j)
				}
			}
		})
	}
}
