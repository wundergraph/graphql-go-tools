package engine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/wundergraph/cosmo/router/gen/proto/wg/cosmo/common"
	nodev1 "github.com/wundergraph/cosmo/router/gen/proto/wg/cosmo/node/v1"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/wundergraph/cosmo/composition-go"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type SubgraphConfiguration struct {
	Name string
	URL  string
	SDL  string

	SubscriptionUrl      string
	SubscriptionProtocol SubscriptionProtocol
}

type SubscriptionProtocol string

const (
	SubscriptionProtocolWS      SubscriptionProtocol = "ws"
	SubscriptionProtocolSSE     SubscriptionProtocol = "sse"
	SubscriptionProtocolSSEPost SubscriptionProtocol = "sse_post"
)

type federationEngineConfigFactoryOptions struct {
	httpClient                *http.Client
	streamingClient           *http.Client
	subscriptionClientFactory graphql_datasource.GraphQLSubscriptionClientFactory
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

func WithFederationSubscriptionClientFactory(factory graphql_datasource.GraphQLSubscriptionClientFactory) FederationEngineConfigFactoryOption {
	return func(options *federationEngineConfigFactoryOptions) {
		options.subscriptionClientFactory = factory
	}
}

func WithFederationSubscriptionType(subscriptionType SubscriptionType) FederationEngineConfigFactoryOption {
	return func(options *federationEngineConfigFactoryOptions) {
		options.subscriptionType = subscriptionType
	}
}

func NewFederationEngineConfigFactory(engineCtx context.Context, subgraphsConfigs []SubgraphConfiguration, opts ...FederationEngineConfigFactoryOption) *FederationEngineConfigFactory {
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
		subscriptionClientFactory: &graphql_datasource.DefaultSubscriptionClientFactory{},
		subscriptionType:          SubscriptionTypeUnknown,
	}

	for _, optFunc := range opts {
		optFunc(&options)
	}

	return &FederationEngineConfigFactory{
		engineCtx:                 engineCtx,
		httpClient:                options.httpClient,
		streamingClient:           options.streamingClient,
		subscriptionClientFactory: options.subscriptionClientFactory,
		subscriptionType:          options.subscriptionType,
		customResolveMap:          options.customResolveMap,
		subgraphsConfigs:          subgraphsConfigs,
	}
}

// FederationEngineConfigFactory is used to create an engine config for a supergraph with multiple data sources for subgraphs.
type FederationEngineConfigFactory struct {
	engineCtx                 context.Context
	httpClient                *http.Client
	streamingClient           *http.Client
	schema                    *graphql.Schema
	subscriptionClientFactory graphql_datasource.GraphQLSubscriptionClientFactory
	subscriptionType          SubscriptionType
	customResolveMap          map[string]resolve.CustomResolve
	subgraphsConfigs          []SubgraphConfiguration
}

func (f *FederationEngineConfigFactory) BuildEngineConfiguration() (conf Configuration, err error) {

	intermediateConfig, err := f.compose()
	if err != nil {
		return Configuration{}, err
	}

	plannerConfiguration, err := f.createPlannerConfiguration(intermediateConfig)
	if err != nil {
		return Configuration{}, err
	}
	plannerConfiguration.DefaultFlushIntervalMillis = DefaultFlushIntervalInMilliseconds

	schemaSDL := intermediateConfig.EngineConfig.GraphqlSchema

	schema, err := graphql.NewSchemaFromString(schemaSDL)
	if err != nil {
		return Configuration{}, err
	}

	conf = Configuration{
		plannerConfig: *plannerConfiguration,
		schema:        schema,
	}

	if f.customResolveMap != nil {
		conf.SetCustomResolveMap(f.customResolveMap)
	}

	return conf, nil
}

func (f *FederationEngineConfigFactory) compose() (*nodev1.RouterConfig, error) {
	subgraphs := make([]*composition.Subgraph, len(f.subgraphsConfigs))

	for i, subgraphConfig := range f.subgraphsConfigs {
		subgraphs[i] = &composition.Subgraph{
			Name:   subgraphConfig.Name,
			URL:    subgraphConfig.URL,
			Schema: subgraphConfig.SDL,
		}

		if subgraphConfig.SubscriptionUrl != "" {
			subgraphs[i].SubscriptionURL = subgraphConfig.SubscriptionUrl
		}

		if subgraphConfig.SubscriptionProtocol == "" {
			subgraphs[i].SubscriptionProtocol = string(SubscriptionProtocolWS)
		} else {
			subgraphs[i].SubscriptionProtocol = string(subgraphConfig.SubscriptionProtocol)
		}
	}

	resultJSON, err := composition.BuildRouterConfiguration(subgraphs...)
	if err != nil {
		return nil, err
	}

	var routerConfig nodev1.RouterConfig
	if err := protojson.Unmarshal([]byte(resultJSON), &routerConfig); err != nil {
		return nil, err
	}

	return &routerConfig, nil
}

func (f *FederationEngineConfigFactory) createPlannerConfiguration(routerConfig *nodev1.RouterConfig) (*plan.Configuration, error) {
	var (
		outConfig plan.Configuration
	)
	// attach field usage information to the plan
	engineConfig := routerConfig.EngineConfig
	// outConfig.IncludeInfo = l.includeInfo
	outConfig.DefaultFlushIntervalMillis = engineConfig.DefaultFlushInterval
	for _, configuration := range engineConfig.FieldConfigurations {
		var args []plan.ArgumentConfiguration
		for _, argumentConfiguration := range configuration.ArgumentsConfiguration {
			arg := plan.ArgumentConfiguration{
				Name: argumentConfiguration.Name,
			}
			switch argumentConfiguration.SourceType {
			case nodev1.ArgumentSource_FIELD_ARGUMENT:
				arg.SourceType = plan.FieldArgumentSource
			case nodev1.ArgumentSource_OBJECT_FIELD:
				arg.SourceType = plan.ObjectFieldSource
			}
			args = append(args, arg)
		}
		outConfig.Fields = append(outConfig.Fields, plan.FieldConfiguration{
			TypeName:  configuration.TypeName,
			FieldName: configuration.FieldName,
			Arguments: args,
		})
	}

	for _, configuration := range engineConfig.TypeConfigurations {
		outConfig.Types = append(outConfig.Types, plan.TypeConfiguration{
			TypeName: configuration.TypeName,
			RenameTo: configuration.RenameTo,
		})
	}

	for _, ds := range engineConfig.DatasourceConfigurations {
		if ds.Kind != nodev1.DataSourceKind_GRAPHQL {
			return nil, fmt.Errorf("invalid datasource kind %q", ds.Kind)
		}

		dataSource, err := f.subgraphDataSourceConfiguration(engineConfig, ds)
		if err != nil {
			return nil, fmt.Errorf("failed to create data source configuration for data source %s: %w", ds.Id, err)
		}

		outConfig.DataSources = append(outConfig.DataSources, dataSource)
	}

	return &outConfig, nil
}

func (f *FederationEngineConfigFactory) subgraphDataSourceConfiguration(engineConfig *nodev1.EngineConfiguration, in *nodev1.DataSourceConfiguration) (plan.DataSource, error) {
	var out plan.DataSource

	factory, err := f.graphqlDataSourceFactory()
	if err != nil {
		return nil, err
	}

	header := http.Header{}
	for s, httpHeader := range in.CustomGraphql.Fetch.Header {
		for _, value := range httpHeader.Values {
			header.Add(s, LoadStringVariable(value))
		}
	}

	fetchUrl := LoadStringVariable(in.CustomGraphql.Fetch.GetUrl())

	subscriptionUrl := LoadStringVariable(in.CustomGraphql.Subscription.Url)
	if subscriptionUrl == "" {
		subscriptionUrl = fetchUrl
	}

	customScalarTypeFields := make([]graphql_datasource.SingleTypeField, len(in.CustomGraphql.CustomScalarTypeFields))
	for i, v := range in.CustomGraphql.CustomScalarTypeFields {
		customScalarTypeFields[i] = graphql_datasource.SingleTypeField{
			TypeName:  v.TypeName,
			FieldName: v.FieldName,
		}
	}

	graphqlSchema, err := f.LoadInternedString(engineConfig, in.CustomGraphql.GetUpstreamSchema())
	if err != nil {
		return nil, fmt.Errorf("could not load GraphQL schema for data source %s: %w", in.Id, err)
	}

	var subscriptionUseSSE bool
	var subscriptionSSEMethodPost bool
	if in.CustomGraphql.Subscription.Protocol != nil {
		switch *in.CustomGraphql.Subscription.Protocol {
		case common.GraphQLSubscriptionProtocol_GRAPHQL_SUBSCRIPTION_PROTOCOL_WS:
			subscriptionUseSSE = false
			subscriptionSSEMethodPost = false
		case common.GraphQLSubscriptionProtocol_GRAPHQL_SUBSCRIPTION_PROTOCOL_SSE:
			subscriptionUseSSE = true
			subscriptionSSEMethodPost = false
		case common.GraphQLSubscriptionProtocol_GRAPHQL_SUBSCRIPTION_PROTOCOL_SSE_POST:
			subscriptionUseSSE = true
			subscriptionSSEMethodPost = true
		}
	} else {
		// Old style config
		if in.CustomGraphql.Subscription.UseSSE != nil {
			subscriptionUseSSE = *in.CustomGraphql.Subscription.UseSSE
		}
	}
	// dataSourceRules := FetchURLRules(&routerEngineConfig.Headers, routerConfig.Subgraphs, subscriptionUrl)
	// forwardedClientHeaders, forwardedClientRegexps, err := PropagatedHeaders(dataSourceRules)
	// if err != nil {
	// 	return nil, fmt.Errorf("error parsing header rules for data source %s: %w", in.Id, err)
	// }

	schemaConfiguration, err := graphql_datasource.NewSchemaConfiguration(
		graphqlSchema,
		&graphql_datasource.FederationConfiguration{
			Enabled:    in.CustomGraphql.Federation.Enabled,
			ServiceSDL: in.CustomGraphql.Federation.ServiceSdl,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("error creating schema configuration for data source %s: %w", in.Id, err)
	}

	customConfiguration, err := graphql_datasource.NewConfiguration(graphql_datasource.ConfigurationInput{
		Fetch: &graphql_datasource.FetchConfiguration{
			URL:    fetchUrl,
			Method: in.CustomGraphql.Fetch.Method.String(),
			Header: header,
		},
		Subscription: &graphql_datasource.SubscriptionConfiguration{
			URL:           subscriptionUrl,
			UseSSE:        subscriptionUseSSE,
			SSEMethodPost: subscriptionSSEMethodPost,
			// ForwardedClientHeaderNames:              forwardedClientHeaders,
			// ForwardedClientHeaderRegularExpressions: forwardedClientRegexps,
		},
		SchemaConfiguration:    schemaConfiguration,
		CustomScalarTypeFields: customScalarTypeFields,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating custom configuration for data source %s: %w", in.Id, err)
	}

	out, err = plan.NewDataSourceConfiguration[graphql_datasource.Configuration](
		in.Id,
		factory,
		f.dataSourceMetaData(in),
		customConfiguration,
	)
	if err != nil {
		return nil, fmt.Errorf("error creating data source configuration for data source %s: %w", in.Id, err)
	}

	return out, nil
}

func (f *FederationEngineConfigFactory) dataSourceMetaData(in *nodev1.DataSourceConfiguration) *plan.DataSourceMetadata {
	var d plan.DirectiveConfigurations = make([]plan.DirectiveConfiguration, 0, len(in.Directives))

	out := &plan.DataSourceMetadata{
		RootNodes:  make([]plan.TypeField, 0, len(in.RootNodes)),
		ChildNodes: make([]plan.TypeField, 0, len(in.ChildNodes)),
		Directives: &d,
		FederationMetaData: plan.FederationMetaData{
			Keys:     make([]plan.FederationFieldConfiguration, 0, len(in.Keys)),
			Requires: make([]plan.FederationFieldConfiguration, 0, len(in.Requires)),
			Provides: make([]plan.FederationFieldConfiguration, 0, len(in.Provides)),
		},
	}

	for _, node := range in.RootNodes {
		out.RootNodes = append(out.RootNodes, plan.TypeField{
			TypeName:   node.TypeName,
			FieldNames: node.FieldNames,
		})
	}
	for _, node := range in.ChildNodes {
		out.ChildNodes = append(out.ChildNodes, plan.TypeField{
			TypeName:   node.TypeName,
			FieldNames: node.FieldNames,
		})
	}
	for _, directive := range in.Directives {
		*out.Directives = append(*out.Directives, plan.DirectiveConfiguration{
			DirectiveName: directive.DirectiveName,
			RenameTo:      directive.DirectiveName,
		})
	}

	for _, keyConfiguration := range in.Keys {
		out.FederationMetaData.Keys = append(out.FederationMetaData.Keys, plan.FederationFieldConfiguration{
			TypeName:     keyConfiguration.TypeName,
			FieldName:    keyConfiguration.FieldName,
			SelectionSet: keyConfiguration.SelectionSet,
		})
	}
	for _, providesConfiguration := range in.Provides {
		out.FederationMetaData.Provides = append(out.FederationMetaData.Provides, plan.FederationFieldConfiguration{
			TypeName:     providesConfiguration.TypeName,
			FieldName:    providesConfiguration.FieldName,
			SelectionSet: providesConfiguration.SelectionSet,
		})
	}
	for _, requiresConfiguration := range in.Requires {
		out.FederationMetaData.Requires = append(out.FederationMetaData.Requires, plan.FederationFieldConfiguration{
			TypeName:     requiresConfiguration.TypeName,
			FieldName:    requiresConfiguration.FieldName,
			SelectionSet: requiresConfiguration.SelectionSet,
		})
	}
	for _, entityInterfacesConfiguration := range in.EntityInterfaces {
		out.FederationMetaData.EntityInterfaces = append(out.FederationMetaData.EntityInterfaces, plan.EntityInterfaceConfiguration{
			InterfaceTypeName: entityInterfacesConfiguration.InterfaceTypeName,
			ConcreteTypeNames: entityInterfacesConfiguration.ConcreteTypeNames,
		})
	}
	for _, interfaceObjectConfiguration := range in.InterfaceObjects {
		out.FederationMetaData.InterfaceObjects = append(out.FederationMetaData.InterfaceObjects, plan.EntityInterfaceConfiguration{
			InterfaceTypeName: interfaceObjectConfiguration.InterfaceTypeName,
			ConcreteTypeNames: interfaceObjectConfiguration.ConcreteTypeNames,
		})
	}

	return out
}

func (f *FederationEngineConfigFactory) LoadInternedString(engineConfig *nodev1.EngineConfiguration, str *nodev1.InternedString) (string, error) {
	key := str.GetKey()
	s, ok := engineConfig.StringStorage[key]
	if !ok {
		return "", fmt.Errorf("no string found for key %q", key)
	}
	return s, nil
}

func (f *FederationEngineConfigFactory) graphqlDataSourceFactory() (plan.PlannerFactory[graphql_datasource.Configuration], error) {
	subscriptionClient, err := f.subscriptionClient(f.httpClient, f.streamingClient, f.subscriptionType, f.subscriptionClientFactory)
	if err != nil {
		return nil, err
	}

	return graphql_datasource.NewFactory(
		f.engineCtx,
		f.httpClient,
		subscriptionClient,
	)
}

func (f *FederationEngineConfigFactory) subscriptionClient(
	httpClient *http.Client,
	streamingClient *http.Client,
	subscriptionType SubscriptionType,
	subscriptionClientFactory graphql_datasource.GraphQLSubscriptionClientFactory,
) (*graphql_datasource.SubscriptionClient, error) {
	var graphqlSubscriptionClient graphql_datasource.GraphQLSubscriptionClient
	switch subscriptionType {
	case SubscriptionTypeGraphQLTransportWS:
		graphqlSubscriptionClient = subscriptionClientFactory.NewSubscriptionClient(
			httpClient,
			streamingClient,
			f.engineCtx,
			graphql_datasource.WithWSSubProtocol(graphql_datasource.ProtocolGraphQLTWS),
		)
	default:
		// for compatibility reasons we fall back to graphql-ws protocol
		graphqlSubscriptionClient = subscriptionClientFactory.NewSubscriptionClient(
			httpClient,
			streamingClient,
			f.engineCtx,
			graphql_datasource.WithWSSubProtocol(graphql_datasource.ProtocolGraphQLWS),
		)
	}

	subscriptionClient, ok := graphqlSubscriptionClient.(*graphql_datasource.SubscriptionClient)
	if !ok {
		return nil, errors.New("invalid SubscriptionClient was instantiated")
	}
	return subscriptionClient, nil
}
