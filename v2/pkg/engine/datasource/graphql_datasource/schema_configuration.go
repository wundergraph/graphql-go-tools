package graphql_datasource

import (
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/federation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type SchemaConfiguration interface {
	UpstreamSchema() *ast.Document
	FederationServiceSDL() string
	IsFederationEnabled() bool
}

type FederationConfiguration struct {
	Enabled    bool
	ServiceSDL string
}

type schemaConfiguration struct {
	upstreamSchema    string
	upstreamSchemaAst *ast.Document
	Federation        *FederationConfiguration
}

func (c *schemaConfiguration) UpstreamSchema() *ast.Document {
	return c.upstreamSchemaAst
}

func (c *schemaConfiguration) FederationServiceSDL() string {
	if c.Federation == nil {
		return ""
	}

	return c.Federation.ServiceSDL
}

func (c *schemaConfiguration) IsFederationEnabled() bool {
	return c.Federation != nil && c.Federation.Enabled
}

func NewSchemaConfiguration(upstreamSchema string, federationCfg *FederationConfiguration) (SchemaConfiguration, error) {
	cfg := &schemaConfiguration{upstreamSchema: upstreamSchema, Federation: federationCfg}

	if cfg.upstreamSchema == "" {
		return nil, fmt.Errorf("upstream schema is required")
	}

	definition := ast.NewSmallDocument()
	definitionParser := astparser.NewParser()
	report := &operationreport.Report{}

	if cfg.Federation != nil && cfg.Federation.Enabled {
		if cfg.Federation.ServiceSDL == "" {
			return nil, fmt.Errorf("federation service SDL is required")
		}

		federationSchema, err := federation.BuildFederationSchema(cfg.upstreamSchema, cfg.Federation.ServiceSDL)
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
