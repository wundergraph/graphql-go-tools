package introspection_datasource

import (
	"errors"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/introspection"
)

type Planner struct {
	introspectionData       *introspection.Data
	v                       *plan.Visitor
	rootField               int
	rootFieldName           string
	dataSourceConfiguration plan.DataSourceConfiguration
}

func (p *Planner) Register(visitor *plan.Visitor, dataSourceConfiguration plan.DataSourceConfiguration, _ bool) error {
	p.v = visitor
	p.rootField = ast.InvalidRef
	p.dataSourceConfiguration = dataSourceConfiguration
	visitor.Walker.RegisterEnterFieldVisitor(p)
	return nil
}

func (p *Planner) DownstreamResponseFieldAlias(_ int) (alias string, exists bool) {
	// the Introspection DataSourcePlanner doesn't rewrite upstream fields: skip
	return
}

func (p *Planner) DataSourcePlanningBehavior() plan.DataSourcePlanningBehavior {
	return plan.DataSourcePlanningBehavior{
		MergeAliasedRootNodes:      false,
		OverrideFieldPathFromAlias: true,
	}
}

func (p *Planner) EnterField(ref int) {
	fieldName := p.v.Operation.FieldNameString(ref)
	switch fieldName {
	case typeFieldName, fieldsFieldName, enumValuesFieldName, schemaFieldName:
		p.rootField = ref
		p.rootFieldName = fieldName
	}
}

func (p *Planner) configureInput() string {
	return buildInput(p.rootFieldName)
}

func (p *Planner) ConfigureFetch() plan.FetchConfiguration {
	if p.rootField == ast.InvalidRef {
		p.v.Walker.StopWithInternalErr(errors.New("introspection root field is not set"))
	}

	requiresParallelListItemFetch := false
	switch p.rootFieldName {
	case fieldsFieldName, enumValuesFieldName:
		requiresParallelListItemFetch = p.dataSourceConfiguration.ParentInfo.InsideArray
	}

	return plan.FetchConfiguration{
		Input:                         p.configureInput(),
		RequiresParallelListItemFetch: requiresParallelListItemFetch,
		DataSource: &Source{
			introspectionData: p.introspectionData,
		},
	}
}

func (p *Planner) ConfigureSubscription() plan.SubscriptionConfiguration {
	// the Introspection DataSourcePlanner doesn't have subscription
	return plan.SubscriptionConfiguration{}
}
