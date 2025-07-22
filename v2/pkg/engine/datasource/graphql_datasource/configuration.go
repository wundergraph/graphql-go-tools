package graphql_datasource

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	grpcdatasource "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/grpc_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/federation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type ConfigurationInput struct {
	Fetch                  *FetchConfiguration
	Subscription           *SubscriptionConfiguration
	SchemaConfiguration    *SchemaConfiguration
	CustomScalarTypeFields []SingleTypeField

	GRPC *grpcdatasource.GRPCConfiguration
}

type Configuration struct {
	fetch                  *FetchConfiguration
	subscription           *SubscriptionConfiguration
	schemaConfiguration    SchemaConfiguration
	customScalarTypeFields []SingleTypeField

	grpc *grpcdatasource.GRPCConfiguration
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

	if input.Fetch == nil && input.Subscription == nil && input.GRPC == nil {
		return Configuration{}, errors.New("fetch or subscription or grpc configuration is required")
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

	if input.GRPC != nil {
		cfg.grpc = input.GRPC
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

func (c *Configuration) IsGRPC() bool {
	return c.grpc != nil
}

type SingleTypeField struct {
	TypeName  string
	FieldName string
}

// OnSubscriptionStartFn defines a hook function that is called when a subscription starts.
// It receives the resolve context and can return initial events or an error.
// If an error is returned, the subscription will not start.
type OnSubscriptionStartFn func(ctx *resolve.Context, input []byte) ([][]byte, error)

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
	ForwardedClientHeaderRegularExpressions []RegularExpression
	WsSubProtocol                           string
	OnSubscriptionStartFns                  []OnSubscriptionStartFn
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

		// As BuildFederationSchema is already merged with base definitions, we actually don't want to do it again,
		// but as we have to parse the schema again, we also need to make sure to add the typename fields to union types again
		// TODO: find a better way to do this
		typeNamesVisitor := asttransform.NewTypeNameVisitor()
		if err := typeNamesVisitor.ExtendSchema(definition); err != nil {
			return nil, fmt.Errorf("unable to extend federation schema with type names: %v", err)
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
