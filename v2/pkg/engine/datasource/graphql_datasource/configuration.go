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

type Configuration struct {
	Fetch                  FetchConfiguration
	Subscription           SubscriptionConfiguration
	SchemaConfiguration    *SchemaConfiguration
	CustomScalarTypeFields []SingleTypeField
}

func (c *Configuration) UpstreamSchema() (*ast.Document, error) {
	if c.SchemaConfiguration == nil {
		return nil, errors.New("schema configuration is empty")
	}

	if c.SchemaConfiguration.upstreamSchemaAst == nil {
		return nil, errors.New("upstream schema is not parsed")
	}

	return c.SchemaConfiguration.upstreamSchemaAst, nil
}

type SingleTypeField struct {
	TypeName  string
	FieldName string
}

type SubscriptionConfiguration struct {
	URL           string
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
