package graphql_datasource

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/federation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type ConfigurationInput struct {
	Fetch                  *FetchConfiguration
	Subscription           *SubscriptionConfiguration
	SchemaConfiguration    *SchemaConfiguration
	CustomScalarTypeFields []SingleTypeField
}

type GRPCMapping struct {
	// Services maps user-friendly service names to the actual gRPC service names
	Services map[string]string
	// InputArguments maps GraphQL input arguments to the corresponding gRPC input arguments
	InputArguments map[string]FieldMapping
	// QueryRPCs maps GraphQL query fields to the corresponding gRPC RPC configurations
	QueryRPCs RPCConfigMap
	// MutationRPCs maps GraphQL mutation fields to the corresponding gRPC RPC configurations
	MutationRPCs RPCConfigMap
	// SubscriptionRPCs maps GraphQL subscription fields to the corresponding gRPC RPC configurations
	SubscriptionRPCs RPCConfigMap
	// EntityRPCs defines how GraphQL types are resolved as entities using specific RPCs
	EntityRPCs map[string]EntityRPCConfig
	// Fields defines the field mappings between GraphQL types and gRPC messages
	Fields map[string]FieldMapping
}

// RPCConfigMap is a map of RPC names to RPC configurations
type RPCConfigMap map[string]RPCConfig

// RPCConfig defines the configuration for a specific RPC operation
type RPCConfig struct {
	// RPC is the name of the RPC method to call
	RPC string
	// Request is the name of the request message type
	Request string
	// Response is the name of the response message type
	Response string
}

// EntityRPCConfig defines the configuration for entity lookups
type EntityRPCConfig struct {
	// Key is a list of field names that uniquely identify the entity
	Key string
	// RPCConfig is the embedded configuration for the RPC operation
	RPCConfig
}

// FieldMapping defines the mapping between a GraphQL field and a gRPC field
type FieldMapping map[string]string

// ResolveFieldMapping resolves the gRPC field name for a given GraphQL field name and type
func (g *GRPCMapping) ResolveFieldMapping(typeName string, fieldName string) (string, bool) {
	if g == nil || g.Fields == nil {
		return "", false
	}

	fieldMapping, ok := g.Fields[typeName]
	if !ok {
		return "", false
	}

	grpcFieldName, ok := fieldMapping[fieldName]
	return grpcFieldName, ok
}

func (g *GRPCMapping) ResolveInputArgumentMapping(fieldName string, argumentName string) (string, bool) {
	if g == nil || g.InputArguments == nil {
		return "", false
	}

	fieldMapping, ok := g.InputArguments[fieldName]
	if !ok {
		return "", false
	}

	grpcFieldName, ok := fieldMapping[argumentName]
	return grpcFieldName, ok
}

type Configuration struct {
	fetch                  *FetchConfiguration
	subscription           *SubscriptionConfiguration
	schemaConfiguration    SchemaConfiguration
	customScalarTypeFields []SingleTypeField
	grpcMapping            *GRPCMapping
}

func NewConfiguration(input ConfigurationInput) (Configuration, error) {
	cfg := Configuration{
		customScalarTypeFields: input.CustomScalarTypeFields,
	}

	if input.SchemaConfiguration == nil {
		return Configuration{}, errors.New("schema configuration is required")
	}
	if input.SchemaConfiguration.upstreamSchema == "" {
		return Configuration{}, errors.New("schema configuration is invalid: upstream schema is required")
	}

	cfg.schemaConfiguration = *input.SchemaConfiguration

	if input.Fetch == nil && input.Subscription == nil {
		return Configuration{}, errors.New("fetch or subscription configuration is required")
	}

	if input.Fetch != nil {
		cfg.fetch = input.Fetch

		if cfg.fetch.Method == "" {
			cfg.fetch.Method = "POST"
		}
	}

	if input.Subscription != nil {
		cfg.subscription = input.Subscription

		if cfg.fetch != nil {
			if len(cfg.subscription.Header) == 0 && len(cfg.fetch.Header) > 0 {
				cfg.subscription.Header = cfg.fetch.Header
			}

			if cfg.subscription.URL == "" {
				cfg.subscription.URL = cfg.fetch.URL
			}
		}
	}

	return cfg, nil
}

func (c *Configuration) UpstreamSchema() (*ast.Document, error) {
	if c.schemaConfiguration.upstreamSchemaAst == nil {
		return nil, errors.New("upstream schema is not parsed")
	}

	return c.schemaConfiguration.upstreamSchemaAst, nil
}

func (c *Configuration) IsFederationEnabled() bool {
	return c.schemaConfiguration.federation != nil && c.schemaConfiguration.federation.Enabled
}

func (c *Configuration) FederationConfiguration() *FederationConfiguration {
	return c.schemaConfiguration.federation
}

type SingleTypeField struct {
	TypeName  string
	FieldName string
}

type SubscriptionConfiguration struct {
	URL           string
	Header        http.Header
	UseSSE        bool
	SSEMethodPost bool
	// ForwardedClientHeaderNames indicates headers names that might be forwarded from the
	// client to the upstream server. This is used to determine which connections
	// can be multiplexed together, but the subscription engine does not forward
	// these headers by itself.
	ForwardedClientHeaderNames []string
	// ForwardedClientHeaderRegularExpressions regular expressions that if matched to the header
	// name might be forwarded from the client to the upstream server. This is used to determine
	// which connections can be multiplexed together, but the subscription engine does not forward
	// these headers by itself.
	ForwardedClientHeaderRegularExpressions []*regexp.Regexp
	WsSubProtocol                           string
}

type FetchConfiguration struct {
	URL    string
	Method string
	Header http.Header
}

type FederationConfiguration struct {
	Enabled    bool
	ServiceSDL string
}

type SchemaConfiguration struct {
	upstreamSchema    string
	upstreamSchemaAst *ast.Document
	federation        *FederationConfiguration
}

func (c *SchemaConfiguration) FederationServiceSDL() string {
	if c.federation == nil {
		return ""
	}

	return c.federation.ServiceSDL
}

func (c *SchemaConfiguration) IsFederationEnabled() bool {
	return c.federation != nil && c.federation.Enabled
}

func NewSchemaConfiguration(upstreamSchema string, federationCfg *FederationConfiguration) (*SchemaConfiguration, error) {
	cfg := &SchemaConfiguration{upstreamSchema: upstreamSchema, federation: federationCfg}

	if cfg.upstreamSchema == "" {
		return nil, fmt.Errorf("upstream schema is required")
	}

	definition := ast.NewSmallDocument()
	definitionParser := astparser.NewParser()
	report := &operationreport.Report{}

	if cfg.federation != nil && cfg.federation.Enabled {
		if cfg.federation.ServiceSDL == "" {
			return nil, fmt.Errorf("federation service SDL is required")
		}

		federationSchema, err := federation.BuildFederationSchema(cfg.upstreamSchema, cfg.federation.ServiceSDL)
		if err != nil {
			return nil, fmt.Errorf("unable to build federation schema: %v", err)
		}
		definition.Input.ResetInputString(federationSchema)
		definitionParser.Parse(definition, report)
		if report.HasErrors() {
			return nil, fmt.Errorf("unable to parse federation schema: %v", report)
		}
	} else {
		definition.Input.ResetInputString(cfg.upstreamSchema)
		definitionParser.Parse(definition, report)
		if report.HasErrors() {
			return nil, fmt.Errorf("unable to parse upstream schema: %v", report)
		}

		if err := asttransform.MergeDefinitionWithBaseSchema(definition); err != nil {
			return nil, fmt.Errorf("unable to merge upstream schema with base schema: %v", err)
		}
	}

	cfg.upstreamSchemaAst = definition

	return cfg, nil
}
