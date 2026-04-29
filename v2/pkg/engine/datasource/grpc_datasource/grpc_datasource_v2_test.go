package grpcdatasource

import (
	"context"
	"testing"

	"buf.build/go/hyperpb"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"google.golang.org/protobuf/proto"
	protoref "google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
)

func TestV2ResponseFrameBuilder_MarshalDataEnvelope(t *testing.T) {
	builder := newV2ResponseFrameBuilder()
	root := builder.newObject()
	category := builder.newObject()
	metrics := builder.newArray()

	builder.setObjectField(root, "categories", metrics)
	builder.appendArrayItem(metrics, category)
	builder.setObjectField(category, "id", builder.newString("cat-1"))
	builder.setObjectField(category, "name", builder.newString("Category One"))
	builder.setObjectField(category, "score", builder.newNumber("42"))
	builder.setObjectField(category, "active", builder.newBool(true))

	data := builder.marshalDataEnvelope(root)
	require.JSONEq(t, `{"data":{"categories":[{"id":"cat-1","name":"Category One","score":42,"active":true}]}}`, string(data))
}

func TestNewDataSourceV2_CompilesNativeProgramForSimpleQuery(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	schemaDoc := grpctest.MustGraphQLSchema(t)
	query := `query ComplexFilterTypeQuery($filter: ComplexFilterTypeInput!) { complexFilterType(filter: $filter) { id name } }`
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	require.False(t, report.HasErrors())

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
	require.NoError(t, err)

	ds, err := NewDataSourceV2(conn, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Compiler:     compiler,
		Mapping:      testMapping(),
	})
	require.NoError(t, err)
	require.NotNil(t, ds.program)
	require.True(t, ds.program.nativeOperation)
	require.Len(t, ds.program.stages, 1)
	require.Len(t, ds.program.stages[0].fetches, 1)
}

func TestDataSourceV2_Load_NativeMatchesV1(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	schemaDoc := grpctest.MustGraphQLSchema(t)
	query := `query ComplexFilterTypeQuery($filter: ComplexFilterTypeInput!) { complexFilterType(filter: $filter) { id name } }`
	variables := `{"variables":{"filter":{"filter":{"name":"test","filterField1":"test","filterField2":"test"}}}}`
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	require.False(t, report.HasErrors())

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
	require.NoError(t, err)

	v1, err := NewDataSource(conn, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Compiler:     compiler,
		Mapping:      testMapping(),
	})
	require.NoError(t, err)

	v2, err := NewDataSourceV2(conn, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Compiler:     compiler,
		Mapping:      testMapping(),
	})
	require.NoError(t, err)
	require.True(t, v2.program.nativeOperation)

	input := []byte(`{"query":"` + query + `","body":` + variables + `}`)
	v1Data, err := v1.Load(context.Background(), nil, input)
	require.NoError(t, err)

	v2Data, err := v2.Load(context.Background(), nil, input)
	require.NoError(t, err)

	require.JSONEq(t, string(v1Data), string(v2Data))
}

func TestDataSourceV2_Load_ResolveMatchesV1(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	schemaDoc := grpctest.MustGraphQLSchema(t)
	query := `query CategoriesWithNullableTypes($nullType: String, $valueType: String) { categories { nullMetrics: categoryMetrics(metricType: $nullType) { id metricType value } valueMetrics: categoryMetrics(metricType: $valueType) { id metricType value } } }`
	variables := `{"variables":{"nullType":"unavailable","valueType":"popularity_score"}}`
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	require.False(t, report.HasErrors())

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
	require.NoError(t, err)

	v1, err := NewDataSource(conn, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Compiler:     compiler,
		Mapping:      testMapping(),
	})
	require.NoError(t, err)

	v2, err := NewDataSourceV2(conn, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Compiler:     compiler,
		Mapping:      testMapping(),
	})
	require.NoError(t, err)
	require.True(t, v2.program.nativeOperation)
	require.False(t, v2.program.requiresFallback)
	require.Len(t, v2.program.stages, 2)
	require.Len(t, v2.program.stages[0].fetches, 1)
	require.Len(t, v2.program.stages[1].fetches, 2)

	input := []byte(`{"query":"` + query + `","body":` + variables + `}`)
	v1Data, err := v1.Load(context.Background(), nil, input)
	require.NoError(t, err)

	v2Data, err := v2.Load(context.Background(), nil, input)
	require.NoError(t, err)

	require.JSONEq(t, string(v1Data), string(v2Data))
}

func TestDataSourceV2_LoadValue_ResolveMatchesLoad(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	schemaDoc := grpctest.MustGraphQLSchema(t)
	query := `query CategoriesWithNullableTypes($nullType: String, $valueType: String) { categories { nullMetrics: categoryMetrics(metricType: $nullType) { id metricType value } valueMetrics: categoryMetrics(metricType: $valueType) { id metricType value } } }`
	variables := `{"variables":{"nullType":"unavailable","valueType":"popularity_score"}}`
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	require.False(t, report.HasErrors())

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
	require.NoError(t, err)

	ds, err := NewDataSourceV2(conn, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Compiler:     compiler,
		Mapping:      testMapping(),
	})
	require.NoError(t, err)

	input := []byte(`{"query":"` + query + `","body":` + variables + `}`)
	byteData, err := ds.Load(context.Background(), nil, input)
	require.NoError(t, err)

	value, release, err := ds.LoadValue(context.Background(), nil, input)
	require.NoError(t, err)
	require.NotNil(t, value)
	require.NotNil(t, release)
	defer release()

	nativeData := value.MarshalTo(nil)
	require.JSONEq(t, string(byteData), string(nativeData))
}

func TestDataSourceV2_LoadResult_ResolveMatchesLoadAndLoadValue(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	schemaDoc := grpctest.MustGraphQLSchema(t)
	query := `query CategoriesWithNullableTypes($nullType: String, $valueType: String) { categories { nullMetrics: categoryMetrics(metricType: $nullType) { id metricType value } valueMetrics: categoryMetrics(metricType: $valueType) { id metricType value } } }`
	variables := `{"variables":{"nullType":"unavailable","valueType":"popularity_score"}}`
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	require.False(t, report.HasErrors())

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
	require.NoError(t, err)

	ds, err := NewDataSourceV2(conn, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Compiler:     compiler,
		Mapping:      testMapping(),
	})
	require.NoError(t, err)

	mergeDataSource, ok := any(ds).(resolve.NativeMergeDataSource)
	require.True(t, ok)

	input := []byte(`{"query":"` + query + `","body":` + variables + `}`)
	byteData, err := ds.Load(context.Background(), nil, input)
	require.NoError(t, err)

	value, releaseValue, err := ds.LoadValue(context.Background(), nil, input)
	require.NoError(t, err)
	require.NotNil(t, value)
	require.NotNil(t, releaseValue)
	defer releaseValue()

	result, releaseResult, err := mergeDataSource.LoadResult(context.Background(), nil, input)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, releaseResult)
	defer releaseResult()

	require.JSONEq(t, string(byteData), string(result.MarshalTo(nil)))

	mergeArena := arena.NewMonotonicArena()
	merged, err := result.MergeInto(mergeArena, nil, resolve.PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}}, nil)
	require.NoError(t, err)
	require.JSONEq(t, gjson.GetBytes(byteData, "data").Raw, string(merged.MarshalTo(nil)))
	require.JSONEq(t, string(value.Get("data").MarshalTo(nil)), string(merged.MarshalTo(nil)))
}

func TestDataSourceV2_LoadValue_FederationFanoutMatchesLoad(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	schemaDoc := grpctest.MustGraphQLSchema(t)
	query := `query($representations: [_Any!]!, $input: ShippingEstimateInput!) { _entities(representations: $representations) { ...on Product { id name price shippingEstimate(input: $input) } } }`
	variables := `{"variables":{"representations":[{"__typename":"Product","id":"1"},{"__typename":"Product","id":"2"},{"__typename":"Product","id":"3"}],"input":{"destination":"INTERNATIONAL","weight":10.0,"expedited":true}}}`
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	require.False(t, report.HasErrors())

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
	require.NoError(t, err)

	ds, err := NewDataSourceV2(conn, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Compiler:     compiler,
		Mapping:      testMapping(),
		FederationConfigs: plan.FederationFieldConfigurations{
			{
				TypeName:     "Product",
				SelectionSet: "id",
			},
		},
	})
	require.NoError(t, err)

	input := []byte(`{"query":"` + query + `","body":` + variables + `}`)
	byteData, err := ds.Load(context.Background(), nil, input)
	require.NoError(t, err)

	value, release, err := ds.LoadValue(context.Background(), nil, input)
	require.NoError(t, err)
	require.NotNil(t, value)
	require.NotNil(t, release)
	defer release()

	nativeData := value.MarshalTo(nil)
	require.JSONEq(t, string(byteData), string(nativeData))
}

func TestV2NativeMergeResult_MergeInto_SupportsIndexedSelectPath(t *testing.T) {
	frame := newV2ResponseFrameBuilder()
	root := frame.newObject()
	entities := frame.newArray()
	product := frame.newObject()
	frame.setObjectField(root, "_entities", entities)
	frame.appendArrayItem(entities, product)
	frame.setObjectField(product, "id", frame.newString("1"))
	frame.setObjectField(product, "name", frame.newString("Table"))

	result := &v2NativeMergeResult{frame: frame, root: root}
	mergeArena := arena.NewMonotonicArena()
	merged, err := result.MergeInto(mergeArena, nil, resolve.PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities", "0"}}, nil)
	require.NoError(t, err)
	require.NotNil(t, merged)
	require.JSONEq(t, `{"id":"1","name":"Table"}`, string(merged.MarshalTo(nil)))
}

func TestDataSourceV2_CompilesNativeProgramForFederationFanout(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	schemaDoc := grpctest.MustGraphQLSchema(t)
	query := `query($representations: [_Any!]!, $input: ShippingEstimateInput!) { _entities(representations: $representations) { ...on Product { id name price shippingEstimate(input: $input) } } }`
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	require.False(t, report.HasErrors())

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
	require.NoError(t, err)

	ds, err := NewDataSourceV2(conn, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Compiler:     compiler,
		Mapping:      testMapping(),
		FederationConfigs: plan.FederationFieldConfigurations{
			{
				TypeName:     "Product",
				SelectionSet: "id",
			},
		},
	})
	require.NoError(t, err)
	require.Truef(t, ds.program.nativeOperation, "fallback reasons: %v", ds.program.fallbackReasons)
	require.Falsef(t, ds.program.requiresFallback, "fallback reasons: %v", ds.program.fallbackReasons)
	require.Len(t, ds.program.stages, 2)
	require.Len(t, ds.program.stages[0].fetches, 1)
	require.Len(t, ds.program.stages[1].fetches, 1)
	require.Equal(t, CallKindEntity, ds.program.stages[0].fetches[0].kind)
	require.Equal(t, CallKindResolve, ds.program.stages[1].fetches[0].kind)
}

func TestDataSourceV2_CompilesNativeProgramForFederationRequiresAndUnionResolve(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	schemaDoc := grpctest.MustGraphQLSchema(t)
	query := `query($representations: [_Any!]!, $checkHealth: Boolean!) { _entities(representations: $representations) { ...on Storage { __typename id tagSummary storageStatus(checkHealth: $checkHealth) { ... on ActionSuccess { message timestamp } ... on ActionError { message code } } } } }`
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	require.False(t, report.HasErrors())

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
	require.NoError(t, err)

	ds, err := NewDataSourceV2(conn, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Compiler:     compiler,
		Mapping:      testMapping(),
		FederationConfigs: plan.FederationFieldConfigurations{
			{
				TypeName:     "Storage",
				SelectionSet: "id",
			},
			{
				TypeName:     "Storage",
				FieldName:    "tagSummary",
				SelectionSet: "tags",
			},
		},
	})
	require.NoError(t, err)
	require.Truef(t, ds.program.nativeOperation, "fallback reasons: %v", ds.program.fallbackReasons)
	require.Falsef(t, ds.program.requiresFallback, "fallback reasons: %v", ds.program.fallbackReasons)
}

func TestDataSourceV2_LoadValue_FederationRequiresAndUnionResolveMatchesLoad(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	schemaDoc := grpctest.MustGraphQLSchema(t)
	query := `query($representations: [_Any!]!, $checkHealth: Boolean!) { _entities(representations: $representations) { ...on Storage { __typename id tagSummary storageStatus(checkHealth: $checkHealth) { ... on ActionSuccess { message timestamp } ... on ActionError { message code } } } } }`
	variables := `{"variables":{"representations":[{"__typename":"Storage","id":"1","tags":["electronics","gadgets","sale"]},{"__typename":"Storage","id":"2","tags":["books","fiction"]},{"__typename":"Storage","id":"3","tags":[]}],"checkHealth":true}}`
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	require.False(t, report.HasErrors())

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
	require.NoError(t, err)

	ds, err := NewDataSourceV2(conn, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Compiler:     compiler,
		Mapping:      testMapping(),
		FederationConfigs: plan.FederationFieldConfigurations{
			{
				TypeName:     "Storage",
				SelectionSet: "id",
			},
			{
				TypeName:     "Storage",
				FieldName:    "tagSummary",
				SelectionSet: "tags",
			},
		},
	})
	require.NoError(t, err)

	input := []byte(`{"query":"` + query + `","body":` + variables + `}`)
	byteData, err := ds.Load(context.Background(), nil, input)
	require.NoError(t, err)

	value, release, err := ds.LoadValue(context.Background(), nil, input)
	require.NoError(t, err)
	require.NotNil(t, value)
	require.NotNil(t, release)
	defer release()

	nativeData := value.MarshalTo(nil)
	require.JSONEq(t, string(byteData), string(nativeData))
}

func TestDataSourceV2_SchemaRuntimeTracksDynamicAndGeneratedHandles(t *testing.T) {
	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
	require.NoError(t, err)

	runtime, err := newV2SchemaRuntime(compiler)
	require.NoError(t, err)

	categoryRuntime := runtime.messageByName["Category"]
	require.NotNil(t, categoryRuntime)
	require.NotNil(t, categoryRuntime.dynamicType)
	require.NotNil(t, categoryRuntime.desc)

	complexFilterRuntime := runtime.messageByName["QueryComplexFilterTypeRequest"]
	require.NotNil(t, complexFilterRuntime)
	require.NotNil(t, complexFilterRuntime.dynamicType)

	require.NotEmpty(t, runtime.methodByName)
	require.NotEmpty(t, runtime.serviceNamesByMethod)
}

func TestDataSourceV2_NativeProgramBuildsRequestFromVariables(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	schemaDoc := grpctest.MustGraphQLSchema(t)
	query := `query ComplexFilterTypeQuery($filter: ComplexFilterTypeInput!) { complexFilterType(filter: $filter) { id name } }`
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	require.False(t, report.HasErrors())

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
	require.NoError(t, err)

	ds, err := NewDataSourceV2(conn, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Compiler:     compiler,
		Mapping:      testMapping(),
	})
	require.NoError(t, err)

	variables := gjson.Parse(`{"filter":{"filter":{"name":"test","filterField1":"test","filterField2":"test"}}}`)
	req, err := ds.program.stages[0].fetches[0].request.build(variables, ds.schema, ds.fallback.rc)
	require.NoError(t, err)
	require.Equal(t, "QueryComplexFilterTypeRequest", string(req.Descriptor().Name()))
	filterField := req.Descriptor().Fields().ByName("filter")
	require.NotNil(t, filterField)
	require.True(t, req.Has(filterField))
}

func TestDataSourceV2_ResolveProgramBuildsContextRequestFromDependencyOutput(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	schemaDoc := grpctest.MustGraphQLSchema(t)
	query := `query CategoriesWithNullableTypes($nullType: String, $valueType: String) { categories { nullMetrics: categoryMetrics(metricType: $nullType) { id metricType value } valueMetrics: categoryMetrics(metricType: $valueType) { id metricType value } } }`
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	require.False(t, report.HasErrors())

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
	require.NoError(t, err)

	ds, err := NewDataSourceV2(conn, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Compiler:     compiler,
		Mapping:      testMapping(),
	})
	require.NoError(t, err)
	require.True(t, ds.program.nativeOperation)

	var resolveFetch *v2Fetch
	for i := range ds.program.stages[1].fetches {
		fetch := &ds.program.stages[1].fetches[i]
		if fetch.kind == CallKindResolve {
			resolveFetch = fetch
			break
		}
	}
	require.NotNil(t, resolveFetch)

	dependencyRuntime := ds.schema.messageByName["QueryCategoriesResponse"]
	categoryRuntime := ds.schema.messageByName["Category"]
	require.NotNil(t, dependencyRuntime)
	require.NotNil(t, categoryRuntime)

	dependencyOutput := dependencyRuntime.newMessage()
	categoriesField := dependencyOutput.Descriptor().Fields().ByName("categories")
	require.NotNil(t, categoriesField)
	categories := dependencyOutput.Mutable(categoriesField).List()

	appendCategory := func(id, name string) {
		category := categoryRuntime.newMessage()
		category.Set(category.Descriptor().Fields().ByName("id"), protoref.ValueOfString(id))
		category.Set(category.Descriptor().Fields().ByName("name"), protoref.ValueOfString(name))
		categories.Append(protoref.ValueOfMessage(category))
	}
	appendCategory("cat-1", "Category One")
	appendCategory("cat-2", "Category Two")

	variables := gjson.Parse(`{"nullType":"unavailable","valueType":"popularity_score"}`)
	req, skip, err := resolveFetch.request.buildWithDependency(variables, dependencyOutput, ds.schema, ds.fallback.rc)
	require.NoError(t, err)
	require.False(t, skip)
	require.Equal(t, "ResolveCategoryCategoryMetricsRequest", string(req.Descriptor().Name()))

	contextField := req.Descriptor().Fields().ByName("context")
	require.NotNil(t, contextField)
	contextList := req.Get(contextField).List()
	require.Equal(t, 2, contextList.Len())
	require.Equal(t, "cat-1", contextList.Get(0).Message().Get(contextList.Get(0).Message().Descriptor().Fields().ByName("id")).String())
	require.Equal(t, "Category One", contextList.Get(0).Message().Get(contextList.Get(0).Message().Descriptor().Fields().ByName("name")).String())

	fieldArgs := req.Descriptor().Fields().ByName("field_args")
	require.NotNil(t, fieldArgs)
	fieldArgsMessage := req.Get(fieldArgs).Message()
	require.Equal(t, "unavailable", fieldArgsMessage.Get(fieldArgsMessage.Descriptor().Fields().ByName("metric_type")).String())
}

func TestV2MessageRuntime_NewDecodeMessage_UsesHyperpbWhenGeneratedTypeMissing(t *testing.T) {
	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
	require.NoError(t, err)

	runtime, err := newV2SchemaRuntime(compiler)
	require.NoError(t, err)

	messageRuntime := runtime.messageByName["QueryCategoriesResponse"]
	require.NotNil(t, messageRuntime)
	require.NotNil(t, messageRuntime.generatedType)
	require.NotNil(t, messageRuntime.hyperType)

	messageRuntime.generatedType = nil
	messageRuntime.generatedDesc = nil

	shared := new(hyperpb.Shared)
	msg := messageRuntime.newDecodeMessage(shared)
	require.NotNil(t, msg)
	_, ok := msg.(*hyperpb.Message)
	require.True(t, ok)
	shared.Free()
}

func TestV2RequestProgram_BuildInput_UsesWirePlanForNestedRequest(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	schemaDoc := grpctest.MustGraphQLSchema(t)
	query := `query ComplexFilterTypeQuery($filter: ComplexFilterTypeInput!) { complexFilterType(filter: $filter) { id name } }`
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	require.False(t, report.HasErrors())

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
	require.NoError(t, err)

	ds, err := NewDataSourceV2(conn, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Compiler:     compiler,
		Mapping:      testMapping(),
	})
	require.NoError(t, err)

	variables := gjson.Parse(`{"filter":{"filter":{"name":"test","filterField1":"test","filterField2":"test"}}}`)
	input, err := ds.program.stages[0].fetches[0].request.buildInput(variables, ds.schema, ds.fallback.rc)
	require.NoError(t, err)

	wire, ok := input.(*v2PreMarshaledInput)
	require.True(t, ok)
	require.NotEmpty(t, wire.wire)

	inputMessage, ok := ds.fallback.rc.doc.MessageByName("QueryComplexFilterTypeRequest")
	require.True(t, ok)
	decoded := dynamicpb.NewMessage(inputMessage.Desc)
	require.NoError(t, proto.Unmarshal(wire.wire, decoded))

	filterField := decoded.Descriptor().Fields().ByName("filter")
	require.NotNil(t, filterField)
	require.True(t, decoded.Has(filterField))
	complexFilterMessage := decoded.Get(filterField).Message()
	nestedFilterField := complexFilterMessage.Descriptor().Fields().ByName("filter")
	require.NotNil(t, nestedFilterField)
	nestedFilterMessage := complexFilterMessage.Get(nestedFilterField).Message()
	require.Equal(t, "test", nestedFilterMessage.Get(nestedFilterMessage.Descriptor().Fields().ByName("name")).String())
	require.Equal(t, "test", nestedFilterMessage.Get(nestedFilterMessage.Descriptor().Fields().ByName("filter_field_1")).String())
	require.Equal(t, "test", nestedFilterMessage.Get(nestedFilterMessage.Descriptor().Fields().ByName("filter_field_2")).String())
}
