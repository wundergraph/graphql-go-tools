package graphql

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	graphqlDataSource "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/federation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/federation/federationdata"
)

type federationEngineConfigFactoryOptions struct {
	httpClient                *http.Client
	streamingClient           *http.Client
	subscriptionClientFactory graphqlDataSource.GraphQLSubscriptionClientFactory
	subscriptionType          SubscriptionType
	customResolveMap          map[string]resolve.CustomResolve
}

type FederationEngineConfigFactoryOption func(options *federationEngineConfigFactoryOptions)

func WithCustomResolveMap(customResolveMap map[string]resolve.CustomResolve) FederationEngineConfigFactoryOption {
	return func(options *federationEngineConfigFactoryOptions) {
		options.customResolveMap = customResolveMap
	}
}

func WithFederationHttpClient(client *http.Client) FederationEngineConfigFactoryOption {
	return func(options *federationEngineConfigFactoryOptions) {
		options.httpClient = client
	}
}

func WithFederationStreamingClient(client *http.Client) FederationEngineConfigFactoryOption {
	return func(options *federationEngineConfigFactoryOptions) {
		options.streamingClient = client
	}
}

func WithFederationSubscriptionClientFactory(factory graphqlDataSource.GraphQLSubscriptionClientFactory) FederationEngineConfigFactoryOption {
	return func(options *federationEngineConfigFactoryOptions) {
		options.subscriptionClientFactory = factory
	}
}

func WithFederationSubscriptionType(subscriptionType SubscriptionType) FederationEngineConfigFactoryOption {
	return func(options *federationEngineConfigFactoryOptions) {
		options.subscriptionType = subscriptionType
	}
}

type DataSourceConfiguration struct {
	ID            string                          // ID of the data source which is used to identify the data source in the engine.
	Configuration graphqlDataSource.Configuration // Configuration fetch and schema related configuration for the data source.
}

func NewFederationEngineConfigFactory(engineCtx context.Context, dataSourceConfigs []DataSourceConfiguration, opts ...FederationEngineConfigFactoryOption) *FederationEngineConfigFactory {
	options := federationEngineConfigFactoryOptions{
		httpClient: &http.Client{
			Timeout: time.Second * 10,
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 1024,
				TLSHandshakeTimeout: 0 * time.Second,
			},
		},
		streamingClient: &http.Client{
			Timeout: 0,
		},
		subscriptionClientFactory: &graphqlDataSource.DefaultSubscriptionClientFactory{},
		subscriptionType:          SubscriptionTypeUnknown,
	}

	for _, optFunc := range opts {
		optFunc(&options)
	}

	return &FederationEngineConfigFactory{
		engineCtx:                 engineCtx,
		httpClient:                options.httpClient,
		streamingClient:           options.streamingClient,
		dataSourceConfigs:         dataSourceConfigs,
		subscriptionClientFactory: options.subscriptionClientFactory,
		subscriptionType:          options.subscriptionType,
		customResolveMap:          options.customResolveMap,
	}
}

// FederationEngineConfigFactory is used to create a v2 engine config for a supergraph with multiple data sources for subgraphs.
type FederationEngineConfigFactory struct {
	engineCtx                 context.Context
	httpClient                *http.Client
	streamingClient           *http.Client
	dataSourceConfigs         []DataSourceConfiguration
	schema                    *Schema
	subscriptionClientFactory graphqlDataSource.GraphQLSubscriptionClientFactory
	subscriptionType          SubscriptionType
	customResolveMap          map[string]resolve.CustomResolve
}

func (f *FederationEngineConfigFactory) SetMergedSchemaFromString(mergedSchema string) (err error) {
	f.schema, err = NewSchemaFromString(mergedSchema)
	if err != nil {
		return fmt.Errorf("set merged schema in FederationEngineConfigFactory: %s", err.Error())
	}
	return nil
}

func (f *FederationEngineConfigFactory) MergedSchema() (*Schema, error) {
	if f.schema != nil {
		return f.schema, nil
	}

	SDLs := make([]string, len(f.dataSourceConfigs))
	for i := range f.dataSourceConfigs {
		federationConfiguration := f.dataSourceConfigs[i].Configuration.FederationConfiguration()
		if federationConfiguration == nil {
			return nil, fmt.Errorf("federation configuration is missing for data source %s", f.dataSourceConfigs[i].ID)
		}

		SDLs[i] = federationConfiguration.ServiceSDL
	}

	rawBaseSchema, err := federation.BuildBaseSchemaDocument(SDLs...)
	if err != nil {
		return nil, fmt.Errorf("build base schema: %w", err)
	}

	if f.schema, err = NewSchemaFromString(rawBaseSchema); err != nil {
		return nil, fmt.Errorf("parse schema from string: %v", err)
	}

	return f.schema, nil
}

func (f *FederationEngineConfigFactory) EngineV2Configuration() (conf EngineV2Configuration, err error) {
	schema, err := f.MergedSchema()
	if err != nil {
		return conf, fmt.Errorf("get schema: %v", err)
	}

	conf = NewEngineV2Configuration(schema)

	fieldConfigs, err := f.engineConfigFieldConfigs(schema)
	if err != nil {
		return conf, fmt.Errorf("create field configs: %v", err)
	}

	dataSources, err := f.engineConfigDataSources()
	if err != nil {
		return conf, fmt.Errorf("create datasource config: %v", err)
	}

	conf.SetFieldConfigurations(fieldConfigs)
	conf.SetDataSources(dataSources)

	if f.customResolveMap != nil {
		conf.SetCustomResolveMap(f.customResolveMap)
	}

	return conf, nil
}

func (f *FederationEngineConfigFactory) engineConfigFieldConfigs(schema *Schema) (plan.FieldConfigurations, error) {
	var planFieldConfigs plan.FieldConfigurations

	for _, dataSourceConfig := range f.dataSourceConfigs {
		federationConfiguration := dataSourceConfig.Configuration.FederationConfiguration()
		if federationConfiguration == nil {
			return nil, fmt.Errorf("federation configuration is missing for data source %s", dataSourceConfig.ID)
		}

		doc, report := astparser.ParseGraphqlDocumentString(federationConfiguration.ServiceSDL)
		if report.HasErrors() {
			return nil, fmt.Errorf("parse graphql document string: %s", report.Error())
		}
		extractor := federationdata.NewRequiredFieldExtractor(&doc)
		planFieldConfigs = append(planFieldConfigs, extractor.GetAllRequiredFields()...)
	}

	planFieldConfigs = newGraphQLFieldConfigsV2Generator(schema).Generate(planFieldConfigs...)
	return planFieldConfigs, nil
}

func (f *FederationEngineConfigFactory) engineConfigDataSources() (planDataSources []plan.DataSource, err error) {
	for _, dataSourceConfig := range f.dataSourceConfigs {
		federationConfiguration := dataSourceConfig.Configuration.FederationConfiguration()
		if federationConfiguration == nil {
			return nil, fmt.Errorf("federation configuration is missing for data source %s", dataSourceConfig.ID)
		}

		doc, report := astparser.ParseGraphqlDocumentString(federationConfiguration.ServiceSDL)
		if report.HasErrors() {
			return nil, fmt.Errorf("parse graphql document string: %s", report.Error())
		}

		planDataSource, err := newGraphQLDataSourceV2Generator(f.engineCtx, &doc).Generate(
			dataSourceConfig.ID,
			dataSourceConfig.Configuration,
			f.httpClient,
			WithDataSourceV2GeneratorSubscriptionConfiguration(f.streamingClient, f.subscriptionType),
			WithDataSourceV2GeneratorSubscriptionClientFactory(f.subscriptionClientFactory),
		)
		if err != nil {
			return nil, err
		}

		planDataSources = append(planDataSources, planDataSource)
	}

	return
}
