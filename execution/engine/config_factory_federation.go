package engine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/wundergraph/cosmo/composition-go"
	"github.com/wundergraph/cosmo/router/gen/proto/wg/cosmo/common"
	nodev1 "github.com/wundergraph/cosmo/router/gen/proto/wg/cosmo/node/v1"

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

// SubgraphCachingConfig defines L2 caching configuration for a specific subgraph.
// This allows fine-grained control over which entities and root fields are cached per subgraph.
type SubgraphCachingConfig struct {
	SubgraphName                 string                                          // Name of the subgraph (must match SubgraphConfiguration.Name)
	EntityCaching                plan.EntityCacheConfigurations                  // Caching config for entity types in this subgraph
	RootFieldCaching             plan.RootFieldCacheConfigurations               // Caching config for root fields in this subgraph
	MutationFieldCaching         plan.MutationFieldCacheConfigurations           // Caching config for mutation field behavior in this subgraph
	SubscriptionEntityPopulation plan.SubscriptionEntityPopulationConfigurations // Caching config for subscription entity population/invalidation
	MutationCacheInvalidation    plan.MutationCacheInvalidationConfigurations    // Caching config for mutation-triggered cache invalidation
}

// SubgraphCachingConfigs is a list of per-subgraph caching configurations.
type SubgraphCachingConfigs []SubgraphCachingConfig

// FindBySubgraphName returns the caching config for the given subgraph name, or nil if not found.
func (c SubgraphCachingConfigs) FindBySubgraphName(name string) *SubgraphCachingConfig {
	for i := range c {
		if c[i].SubgraphName == name {
			return &c[i]
		}
	}
	return nil
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
	subgraphCachingConfigs    SubgraphCachingConfigs

	grpcClient grpc.ClientConnInterface
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

// WithSubgraphEntityCachingConfigs registers per-subgraph caching configuration.
// Each SubgraphCachingConfig specifies the caches that apply to a particular subgraph.
// Despite the historical name, the option carries the full SubgraphCachingConfig:
// EntityCaching, RootFieldCaching, MutationFieldCaching, SubscriptionEntityPopulation,
// and MutationCacheInvalidation.
//
// Example:
//
//	WithSubgraphEntityCachingConfigs(SubgraphCachingConfigs{
//	    {
//	        SubgraphName: "products",
//	        EntityCaching: plan.EntityCacheConfigurations{
//	            {TypeName: "Product", CacheName: "default", TTL: 30 * time.Second},
//	        },
//	    },
//	    {
//	        SubgraphName: "accounts",
//	        EntityCaching: plan.EntityCacheConfigurations{
//	            {TypeName: "User", CacheName: "default", TTL: 60 * time.Second},
//	        },
//	    },
//	})
func WithSubgraphEntityCachingConfigs(configs SubgraphCachingConfigs) FederationEngineConfigFactoryOption {
	return func(options *federationEngineConfigFactoryOptions) {
		options.subgraphCachingConfigs = configs
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
		grpcClient: nil,
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
		grpcClient:                options.grpcClient,
		streamingClient:           options.streamingClient,
		subscriptionClientFactory: options.subscriptionClientFactory,
		subscriptionType:          options.subscriptionType,
		customResolveMap:          options.customResolveMap,
		subgraphCachingConfigs:    options.subgraphCachingConfigs,
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
	subgraphCachingConfigs    SubgraphCachingConfigs
	subgraphsConfigs          []SubgraphConfiguration

	grpcClient grpc.ClientConnInterface
}

func (f *FederationEngineConfigFactory) BuildEngineConfiguration() (Configuration, error) {
	rc, err := f.Compose()
	if err != nil {
		return Configuration{}, err
	}
	return f.buildEngineConfiguration(rc)
}

func (f *FederationEngineConfigFactory) BuildEngineConfigurationWithRouterConfig(c *nodev1.RouterConfig) (Configuration, error) {
	if c == nil {
		return Configuration{}, errors.New("router config is nil")
	}
	if c.EngineConfig == nil {
		return Configuration{}, errors.New("router config engine config is nil")
	}
	return f.buildEngineConfiguration(c)
}

func (f *FederationEngineConfigFactory) buildEngineConfiguration(routerConfig *nodev1.RouterConfig) (Configuration, error) {
	plannerConfiguration, err := f.createPlannerConfiguration(routerConfig)
	if err != nil {
		return Configuration{}, err
	}
	plannerConfiguration.DefaultFlushIntervalMillis = DefaultFlushIntervalInMilliseconds
	schemaSDL := routerConfig.EngineConfig.GraphqlSchema

	schema, err := graphql.NewSchemaFromString(schemaSDL)
	if err != nil {
		return Configuration{}, err
	}

	conf := Configuration{
		plannerConfig: *plannerConfiguration,
		schema:        schema,
	}

	if f.customResolveMap != nil {
		conf.SetCustomResolveMap(f.customResolveMap)
	}

	return conf, nil
}

// Compose produces a federated router configuration.
func (f *FederationEngineConfigFactory) Compose() (*nodev1.RouterConfig, error) {
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

	// Create a mapping from datasource ID to subgraph name.
	// The composition library generates datasources in the same order as
	// subgraphsConfigs entries, indexed by stringified position.
	dsIDToSubgraphName := make(map[string]string)
	for i, subgraphConfig := range f.subgraphsConfigs {
		dsIDToSubgraphName[fmt.Sprintf("%d", i)] = subgraphConfig.Name
	}
	// Backfill from routerConfig.Subgraphs when subgraphsConfigs is empty or
	// partial. Without this, the static-RouterConfig path (subgraphsConfigs == nil)
	// would name every datasource by its numeric ID, so per-subgraph cache configs
	// keyed by real names like "products" would never match in
	// SubgraphCachingConfigs.FindBySubgraphName.
	for _, sg := range routerConfig.GetSubgraphs() {
		id, name := sg.GetId(), sg.GetName()
		if id == "" || name == "" {
			continue
		}
		if _, ok := dsIDToSubgraphName[id]; !ok {
			dsIDToSubgraphName[id] = name
		}
	}

	for _, ds := range engineConfig.DatasourceConfigurations {
		if ds.Kind != nodev1.DataSourceKind_GRAPHQL {
			return nil, fmt.Errorf("invalid datasource kind %q", ds.Kind)
		}

		// Final fallback to the datasource ID when no SubgraphConfiguration entry
		// and no routerConfig.Subgraphs entry maps it — matches master's
		// NewDataSourceConfiguration default (name = id) so callers don't end up
		// registering datasources with empty names.
		subgraphName := dsIDToSubgraphName[ds.Id]
		if subgraphName == "" {
			subgraphName = ds.Id
		}
		dataSource, err := f.subgraphDataSourceConfiguration(engineConfig, ds, subgraphName)
		if err != nil {
			return nil, fmt.Errorf("failed to create data source configuration for data source %s: %w", ds.Id, err)
		}

		outConfig.DataSources = append(outConfig.DataSources, dataSource)
	}

	return &outConfig, nil
}

func (f *FederationEngineConfigFactory) subgraphDataSourceConfiguration(engineConfig *nodev1.EngineConfiguration, in *nodev1.DataSourceConfiguration, subgraphName string) (plan.DataSource, error) {
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

	out, err = plan.NewDataSourceConfigurationWithName[graphql_datasource.Configuration](
		in.Id,
		subgraphName,
		factory,
		f.dataSourceMetaData(in, subgraphName),
		customConfiguration,
	)
	if err != nil {
		return nil, fmt.Errorf("error creating data source configuration for data source %s: %w", in.Id, err)
	}

	return out, nil
}

func (f *FederationEngineConfigFactory) dataSourceMetaData(in *nodev1.DataSourceConfiguration, subgraphName string) *plan.DataSourceMetadata {
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

	// Add caching configuration for this specific subgraph
	// Look up the caching config by subgraph name for explicit per-subgraph configuration
	subgraphCachingConfig := f.subgraphCachingConfigs.FindBySubgraphName(subgraphName)
	if subgraphCachingConfig != nil {
		out.FederationMetaData.EntityCaching = subgraphCachingConfig.EntityCaching
		out.FederationMetaData.RootFieldCaching = subgraphCachingConfig.RootFieldCaching
		out.FederationMetaData.MutationFieldCaching = subgraphCachingConfig.MutationFieldCaching
		out.FederationMetaData.SubscriptionEntityPopulation = subgraphCachingConfig.SubscriptionEntityPopulation
		out.FederationMetaData.MutationCacheInvalidation = subgraphCachingConfig.MutationCacheInvalidation
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
	_ SubscriptionType,
	subscriptionClientFactory graphql_datasource.GraphQLSubscriptionClientFactory,
) (graphql_datasource.GraphQLSubscriptionClient, error) {
	graphqlSubscriptionClient := subscriptionClientFactory.NewSubscriptionClient(
		f.engineCtx,
		graphql_datasource.WithUpgradeClient(httpClient),
		graphql_datasource.WithStreamingClient(streamingClient),
	)

	ok := graphql_datasource.IsDefaultGraphQLSubscriptionClient(graphqlSubscriptionClient)
	if !ok {
		return nil, errors.New("invalid subscriptionClient was instantiated")
	}
	return graphqlSubscriptionClient, nil
}
