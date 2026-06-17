package plan

// This file contains test helpers and tests specific to monday.com tweaks.
// See mondaytweaks package for the corresponding feature flags.

import (
	"context"
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
)

// fieldVisitorFactory is like FakeFactory but its planner also registers as a
// FieldVisitor so that AllowVisitor(LeaveField, …) is invoked for it. That call
// populates Visitor.fieldPlanners — required for the cost tree to attribute data
// source hashes to fields.
type fieldVisitorFactory struct {
	upstreamSchema *ast.Document
	behavior       *DataSourcePlanningBehavior
}

func (f *fieldVisitorFactory) UpstreamSchema(_ DataSourceConfiguration[any]) (*ast.Document, bool) {
	return f.upstreamSchema, true
}

func (f *fieldVisitorFactory) PlanningBehavior() DataSourcePlanningBehavior {
	if f.behavior == nil {
		return DataSourcePlanningBehavior{}
	}
	return *f.behavior
}

func (f *fieldVisitorFactory) Planner(_ abstractlogger.Logger) DataSourcePlanner[any] {
	source := &StatefulSource{}
	go source.Start()
	return &fieldVisitorPlanner{
		source:         source,
		upstreamSchema: f.upstreamSchema,
		behavior:       f.behavior,
	}
}

func (f *fieldVisitorFactory) Context() context.Context { return context.Background() }

type fieldVisitorPlanner struct {
	id             int
	source         *StatefulSource
	upstreamSchema *ast.Document
	behavior       *DataSourcePlanningBehavior
}

func (p *fieldVisitorPlanner) ID() int      { return p.id }
func (p *fieldVisitorPlanner) SetID(id int) { p.id = id }

func (p *fieldVisitorPlanner) EnterDocument(_, _ *ast.Document) {}
func (p *fieldVisitorPlanner) EnterField(_ int)                 {}
func (p *fieldVisitorPlanner) LeaveField(_ int)                 {}

func (p *fieldVisitorPlanner) Register(visitor *Visitor, _ DataSourceConfiguration[any], _ DataSourcePlannerConfiguration) error {
	visitor.Walker.RegisterEnterDocumentVisitor(p)
	visitor.Walker.RegisterFieldVisitor(p)
	return nil
}

func (p *fieldVisitorPlanner) ConfigureFetch() resolve.FetchConfiguration {
	return resolve.FetchConfiguration{DataSource: &FakeDataSource{source: p.source}}
}

func (p *fieldVisitorPlanner) ConfigureSubscription() SubscriptionConfiguration {
	return SubscriptionConfiguration{}
}

func (p *fieldVisitorPlanner) DataSourcePlanningBehavior() DataSourcePlanningBehavior {
	if p.behavior == nil {
		return DataSourcePlanningBehavior{}
	}
	return *p.behavior
}

func (p *fieldVisitorPlanner) DownstreamResponseFieldAlias(_ int) (string, bool) {
	return "", false
}

// SchemaWithFieldVisitors is like dsBuilder.Schema but the resulting planner also
// registers as a FieldVisitor. This is needed for cost tests: AllowVisitor(LeaveField, …)
// populates Visitor.fieldPlanners — which is what the cost tree uses to assign data
// source hashes to fields. FakeFactory's planner only registers EnterDocument, so
// fieldPlanners stays empty and no hashes are assigned.
func (b *dsBuilder) SchemaWithFieldVisitors(schema string) *dsBuilder {
	def := unsafeparser.ParseGraphqlDocumentString(schema)
	b.ds.factory = &fieldVisitorFactory{
		upstreamSchema: &def,
		behavior:       b.behavior,
	}
	return b
}

// WithCostConfig attaches a DataSourceCostConfig to a dsBuilder.
func (b *dsBuilder) WithCostConfig(cfg *DataSourceCostConfig) *dsBuilder {
	b.ds.DataSourceMetadata.CostConfig = cfg
	return b
}

// planCostCalc runs the full planner pipeline and returns the CostCalculator.
// Requires ComputeCosts: true in config.
func planCostCalc(t *testing.T, schema, query string, config Configuration) *CostCalculator {
	t.Helper()

	def := unsafeparser.ParseGraphqlDocumentString(schema)
	op := unsafeparser.ParseGraphqlDocumentString(query)
	require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&def))

	var report operationreport.Report
	astnormalization.NewNormalizer(true, true).NormalizeOperation(&op, &def, &report)
	astvalidation.DefaultOperationValidator().Validate(&op, &def, &report)
	require.False(t, report.HasErrors(), report.Error())

	p, err := NewPlanner(config)
	require.NoError(t, err)

	result := p.Plan(&op, &def, "", &report)
	require.False(t, report.HasErrors(), report.Error())

	sync, ok := result.(*SynchronousResponsePlan)
	require.True(t, ok)
	require.NotNil(t, sync.CostCalculator)
	return sync.CostCalculator
}

// findNode returns the first cost tree node with the given field coordinates, or nil.
func findNode(root *CostTreeNode, coord FieldCoordinate) *CostTreeNode {
	if root == nil {
		return nil
	}
	if root.fieldCoords == coord {
		return root
	}
	for _, child := range root.children {
		if n := findNode(child, coord); n != nil {
			return n
		}
	}
	return nil
}

// TestCostVisitor_EntityResolutionPlannerDoesNotInflateParentFieldCost verifies that an
// entity-resolution planner for a child field (Team.name in the users subgraph) does not
// inflate the cost of the parent list field (Query.teams in the staging subgraph).
//
// Root cause: when Team.name requires entity resolution, the users planner registers itself as
// a visitor of Query.teams (to walk into the selection set). Without the fix, the cost visitor
// counts it as a second data source for Query.teams and charges ObjectTypeWeight("Team")=1
// per item on top of staging's configured weight — violating the IBM Cost Specification.
//
// With SkipEntityResolutionPlannerCostForParentField=true, getFieldDataSourceHashes filters
// planners that only traverse through a field (HasPathWithFieldRef=false), so Query.teams
// receives exactly one data source hash (staging's), not two.
func TestCostVisitor_EntityResolutionPlannerDoesNotInflateParentFieldCost(t *testing.T) {
	// Merged schema seen by the gateway.
	const mergedSchema = `
		type Query {
			teams: [Team!]!
		}
		type Team {
			id: ID!
			picture_url: String
			name: String!
		}
	`

	// staging subgraph: owns Query.teams, Team.id, Team.picture_url
	const stagingSchema = `
		type Query {
			teams: [Team!]!
		}
		type Team @key(fields: "id") {
			id: ID!
			picture_url: String
		}
	`
	stagingDS := dsb().
		Id("staging").Hash(1).
		WithBehavior(DataSourcePlanningBehavior{AllowPlanningTypeName: true}).
		SchemaWithFieldVisitors(stagingSchema).
		RootNode("Query", "teams").
		RootNode("Team", "id", "picture_url").
		KeysMetadata(FederationFieldConfigurations{
			{TypeName: "Team", SelectionSet: "id"},
		}).
		WithCostConfig(&DataSourceCostConfig{}).
		DS()

	// users subgraph: owns Team.name via entity resolution
	const usersSchema = `
		type Team @key(fields: "id") {
			id: ID! @external
			name: String!
		}
	`
	usersDS := dsb().
		Id("users").Hash(2).
		SchemaWithFieldVisitors(usersSchema).
		RootNode("Team", "id", "name").
		KeysMetadata(FederationFieldConfigurations{
			{TypeName: "Team", SelectionSet: "id"},
		}).
		WithCostConfig(&DataSourceCostConfig{}).
		DS()

	tree := planCostCalc(t, mergedSchema, `query { teams { id picture_url name } }`, Configuration{
		ComputeCosts:              true,
		StaticCostDefaultListSize: 1,
		DataSources:               []DataSource{stagingDS, usersDS},
	}).tree

	teamsNode := findNode(tree, FieldCoordinate{"Query", "teams"})
	require.NotNil(t, teamsNode, "Query.teams node not found in cost tree")

	assert.Len(t, teamsNode.dataSourceHashes, 1,
		"Query.teams should be attributed to exactly one data source (staging); "+
			"the entity-resolution planner for Team.name must not be counted")
}
