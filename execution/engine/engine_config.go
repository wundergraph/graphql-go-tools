package engine

import (
	"context"
	"errors"
	"net/http"

	"google.golang.org/grpc"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

const (
	DefaultFlushIntervalInMilliseconds = 1000
)

type Configuration struct {
	schema                   *graphql.Schema
	plannerConfig            plan.Configuration
	websocketBeforeStartHook WebsocketBeforeStartHook
}

func NewConfiguration(schema *graphql.Schema) Configuration {
	return Configuration{
		schema: schema,
		plannerConfig: plan.Configuration{
			DefaultFlushIntervalMillis: DefaultFlushIntervalInMilliseconds,
			DataSources:                []plan.DataSource{},
			Fields:                     plan.FieldConfigurations{},
		},
	}
}

func (e *Configuration) Schema() *graphql.Schema {
	return e.schema
}

func (e *Configuration) SetCustomResolveMap(customResolveMap map[string]resolve.CustomResolve) {
	e.plannerConfig.CustomResolveMap = customResolveMap
}

func (e *Configuration) AddDataSource(dataSource plan.DataSource) {
	e.plannerConfig.DataSources = append(e.plannerConfig.DataSources, dataSource)
}

func (e *Configuration) SetDataSources(dataSources []plan.DataSource) {
	e.plannerConfig.DataSources = dataSources
}

func (e *Configuration) AddFieldConfiguration(fieldConfig plan.FieldConfiguration) {
	e.plannerConfig.Fields = append(e.plannerConfig.Fields, fieldConfig)
}

func (e *Configuration) SetFieldConfigurations(fieldConfigs plan.FieldConfigurations) {
	e.plannerConfig.Fields = fieldConfigs
}

func (e *Configuration) DataSources() []plan.DataSource {
	return e.plannerConfig.DataSources
}

func (e *Configuration) FieldConfigurations() plan.FieldConfigurations {
	return e.plannerConfig.Fields
}

// SetWebsocketBeforeStartHook - sets before start hook which will be called before processing any operation sent over websockets
func (e *Configuration) SetWebsocketBeforeStartHook(hook WebsocketBeforeStartHook) {
	e.websocketBeforeStartHook = hook
}

type dataSourceGeneratorOptions struct {
	streamingClient           *http.Client
	subscriptionType          SubscriptionType
	subscriptionClientFactory graphql_datasource.GraphQLSubscriptionClientFactory
}

type DataSourceGeneratorOption func(options *dataSourceGeneratorOptions)

func WithDataSourceGeneratorSubscriptionConfiguration(streamingClient *http.Client, subscriptionType SubscriptionType) DataSourceGeneratorOption {
	return func(options *dataSourceGeneratorOptions) {
		options.streamingClient = streamingClient
		options.subscriptionType = subscriptionType
	}
}

func WithDataSourceGeneratorSubscriptionClientFactory(factory graphql_datasource.GraphQLSubscriptionClientFactory) DataSourceGeneratorOption {
	return func(options *dataSourceGeneratorOptions) {
		options.subscriptionClientFactory = factory
	}
}

type graphqlDataSourceGenerator struct {
	document  *ast.Document
	engineCtx context.Context
}

func newGraphQLDataSourceGenerator(engineCtx context.Context, document *ast.Document) *graphqlDataSourceGenerator {
	return &graphqlDataSourceGenerator{
		document:  document,
		engineCtx: engineCtx,
	}
}

func (d *graphqlDataSourceGenerator) Generate(dsID string, config graphql_datasource.Configuration, httpClient *http.Client, grpcClient grpc.ClientConnInterface, options ...DataSourceGeneratorOption) (plan.DataSource, error) {
	extractor := NewLocalTypeFieldExtractor(d.document)
	rootNodes, childNodes := extractor.GetAllNodes()

	definedOptions := &dataSourceGeneratorOptions{
		streamingClient:           &http.Client{Timeout: 0},
		subscriptionType:          SubscriptionTypeUnknown,
		subscriptionClientFactory: &graphql_datasource.DefaultSubscriptionClientFactory{},
	}

	for _, option := range options {
		option(definedOptions)
	}

	subscriptionClient, err := d.generateSubscriptionClient(httpClient, definedOptions)
	if err != nil {
		return nil, err
	}

	var factory plan.PlannerFactory[graphql_datasource.Configuration]

	if config.IsGRPC() {
		factory, err = graphql_datasource.NewFactoryGRPC(
			d.engineCtx,
			grpcClient,
		)
	} else {
		factory, err = graphql_datasource.NewFactory(
			d.engineCtx,
			httpClient,
			subscriptionClient,
		)
	}

	if err != nil {
		return nil, err
	}

	return plan.NewDataSourceConfiguration[graphql_datasource.Configuration](
		dsID,
		factory,
		&plan.DataSourceMetadata{
			RootNodes:  rootNodes,
			ChildNodes: childNodes,
		},
		config,
	)
}

func (d *graphqlDataSourceGenerator) generateSubscriptionClient(httpClient *http.Client, definedOptions *dataSourceGeneratorOptions) (graphql_datasource.GraphQLSubscriptionClient, error) {
	var graphqlSubscriptionClient graphql_datasource.GraphQLSubscriptionClient
	switch definedOptions.subscriptionType {
	case SubscriptionTypeGraphQLTransportWS:
		graphqlSubscriptionClient = definedOptions.subscriptionClientFactory.NewSubscriptionClient(context.TODO(),
			graphql_datasource.WithUpgradeClient(httpClient),
			graphql_datasource.WithStreamingClient(definedOptions.streamingClient),
		)
	default:
		// for compatibility reasons we fall back to graphql-ws protocol
		graphqlSubscriptionClient = definedOptions.subscriptionClientFactory.NewSubscriptionClient(context.TODO(),
			graphql_datasource.WithUpgradeClient(httpClient),
			graphql_datasource.WithStreamingClient(definedOptions.streamingClient),
		)
	}

	ok := graphql_datasource.IsDefaultGraphQLSubscriptionClient(graphqlSubscriptionClient)
	if !ok {
		return nil, errors.New("invalid subscriptionClient was instantiated")
	}
	return graphqlSubscriptionClient, nil
}

type graphqlFieldConfigurationsGenerator struct {
	schema *graphql.Schema
}

func newGraphQLFieldConfigsGenerator(schema *graphql.Schema) *graphqlFieldConfigurationsGenerator {
	return &graphqlFieldConfigurationsGenerator{
		schema: schema,
	}
}

func (g *graphqlFieldConfigurationsGenerator) Generate(predefinedFieldConfigs ...plan.FieldConfiguration) plan.FieldConfigurations {
	var planFieldConfigs plan.FieldConfigurations
	if len(predefinedFieldConfigs) > 0 {
		planFieldConfigs = predefinedFieldConfigs
	}

	generatedArgs := g.schema.GetAllFieldArguments(graphql.NewSkipReservedNamesFunc())
	generatedArgsAsLookupMap := CreateTypeFieldArgumentsLookupMap(generatedArgs)
	g.engineConfigArguments(&planFieldConfigs, generatedArgsAsLookupMap)

	return planFieldConfigs
}

func (g *graphqlFieldConfigurationsGenerator) engineConfigArguments(fieldConfs *plan.FieldConfigurations, generatedArgs map[TypeFieldLookupKey]graphql.TypeFieldArguments) {
	for i := range *fieldConfs {
		if len(generatedArgs) == 0 {
			return
		}

		lookupKey := CreateTypeFieldLookupKey((*fieldConfs)[i].TypeName, (*fieldConfs)[i].FieldName)
		currentArgs, exists := generatedArgs[lookupKey]
		if !exists {
			continue
		}

		(*fieldConfs)[i].Arguments = g.createArgumentConfigurationsForArgumentNames(currentArgs.ArgumentNames)
		delete(generatedArgs, lookupKey)
	}

	for _, genArgs := range generatedArgs {
		*fieldConfs = append(*fieldConfs, plan.FieldConfiguration{
			TypeName:  genArgs.TypeName,
			FieldName: genArgs.FieldName,
			Arguments: g.createArgumentConfigurationsForArgumentNames(genArgs.ArgumentNames),
		})
	}
}

func (g *graphqlFieldConfigurationsGenerator) createArgumentConfigurationsForArgumentNames(argumentNames []string) plan.ArgumentsConfigurations {
	argConfs := plan.ArgumentsConfigurations{}
	for _, argName := range argumentNames {
		argConf := plan.ArgumentConfiguration{
			Name:       argName,
			SourceType: plan.FieldArgumentSource,
		}

		argConfs = append(argConfs, argConf)
	}

	return argConfs
}
