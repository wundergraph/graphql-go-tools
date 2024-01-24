package engine

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	graphqlDataSource "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
)

type proxyEngineConfigFactoryOptions struct {
	dataSourceID              string
	httpClient                *http.Client
	streamingClient           *http.Client
	subscriptionClientFactory graphqlDataSource.GraphQLSubscriptionClientFactory
}

type ProxyEngineConfigFactoryOption func(options *proxyEngineConfigFactoryOptions)

func WithProxyHttpClient(client *http.Client) ProxyEngineConfigFactoryOption {
	return func(options *proxyEngineConfigFactoryOptions) {
		options.httpClient = client
	}
}

func WithDataSourceID(id string) ProxyEngineConfigFactoryOption {
	return func(options *proxyEngineConfigFactoryOptions) {
		options.dataSourceID = id
	}
}

func WithProxyStreamingClient(client *http.Client) ProxyEngineConfigFactoryOption {
	return func(options *proxyEngineConfigFactoryOptions) {
		options.streamingClient = client
	}
}

func WithProxySubscriptionClientFactory(factory graphqlDataSource.GraphQLSubscriptionClientFactory) ProxyEngineConfigFactoryOption {
	return func(options *proxyEngineConfigFactoryOptions) {
		options.subscriptionClientFactory = factory
	}
}

// ProxyUpstreamConfig holds configuration to configure a single data source to a single upstream.
type ProxyUpstreamConfig struct {
	URL              string
	Method           string
	StaticHeaders    http.Header
	SubscriptionType SubscriptionType
}

// ProxyEngineConfigFactory is used to create an engine config with a single upstream and a single data source for this upstream.
type ProxyEngineConfigFactory struct {
	dataSourceID              string
	httpClient                *http.Client
	streamingClient           *http.Client
	schema                    *graphql.Schema
	proxyUpstreamConfig       ProxyUpstreamConfig
	subscriptionClientFactory graphqlDataSource.GraphQLSubscriptionClientFactory
	engineCtx                 context.Context
}

func NewProxyEngineConfigFactory(engineCtx context.Context, schema *graphql.Schema, proxyUpstreamConfig ProxyUpstreamConfig, opts ...ProxyEngineConfigFactoryOption) *ProxyEngineConfigFactory {
	options := proxyEngineConfigFactoryOptions{
		dataSourceID: uuid.New().String(),
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
	}

	for _, optFunc := range opts {
		optFunc(&options)
	}

	return &ProxyEngineConfigFactory{
		engineCtx:                 engineCtx,
		dataSourceID:              options.dataSourceID,
		httpClient:                options.httpClient,
		streamingClient:           options.streamingClient,
		schema:                    schema,
		proxyUpstreamConfig:       proxyUpstreamConfig,
		subscriptionClientFactory: options.subscriptionClientFactory,
	}
}

func (p *ProxyEngineConfigFactory) EngineConfiguration() (Configuration, error) {
	schemaConfiguration, err := graphqlDataSource.NewSchemaConfiguration(string(p.schema.rawInput), nil)
	if err != nil {
		return EngineV2Configuration{}, err
	}

	dataSourceConfig, err := graphqlDataSource.NewConfiguration(graphqlDataSource.ConfigurationInput{
		Fetch: &graphqlDataSource.FetchConfiguration{
			URL:    p.proxyUpstreamConfig.URL,
			Method: p.proxyUpstreamConfig.Method,
			Header: p.proxyUpstreamConfig.StaticHeaders,
		},
		Subscription: &graphqlDataSource.SubscriptionConfiguration{
			URL:    p.proxyUpstreamConfig.URL,
			UseSSE: p.proxyUpstreamConfig.SubscriptionType == SubscriptionTypeSSE,
		},
		SchemaConfiguration: schemaConfiguration,
	})
	if err != nil {
		return EngineV2Configuration{}, err
	}

	conf := NewConfiguration(p.schema)

	rawDoc, report := astparser.ParseGraphqlDocumentBytes(p.schema.Input())
	if report.HasErrors() {
		return Configuration{}, report
	}

	dataSource, err := newGraphQLDataSourceGenerator(p.engineCtx, &rawDoc).Generate(
		p.dataSourceID,
		dataSourceConfig,
		p.httpClient,
		WithDataSourceGeneratorSubscriptionConfiguration(p.streamingClient, p.proxyUpstreamConfig.SubscriptionType),
		WithDataSourceGeneratorSubscriptionClientFactory(p.subscriptionClientFactory),
	)
	if err != nil {
		return Configuration{}, err
	}

	conf.AddDataSource(dataSource)
	fieldConfigs := newGraphQLFieldConfigsGenerator(p.schema).Generate()
	conf.SetFieldConfigurations(fieldConfigs)

	return conf, nil
}
