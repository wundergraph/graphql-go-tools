package introspection_datasource

import (
	"errors"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/introspection"
)

type Planner struct {
	introspectionData *introspection.Data
	v                 *plan.Visitor
	rootField         int
}

func (p *Planner) Register(visitor *plan.Visitor, _ plan.DataSourceConfiguration, _ bool) error {
	p.v = visitor
	p.rootField = ast.InvalidRef
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
		OverrideFieldPathFromAlias: false,
	}
}

func (p *Planner) EnterField(ref int) {
	fieldName := p.v.Operation.FieldNameString(ref)
	switch fieldName {
	case typeFieldName, fieldsFieldName, enumValuesFieldName, schemaFieldName:
		p.rootField = ref
	}
}

func (p *Planner) configureInput() string {
	fieldName := p.v.Operation.FieldNameString(p.rootField)

	return buildInput(fieldName)
}

func (p *Planner) ConfigureFetch() plan.FetchConfiguration {
	if p.rootField == ast.InvalidRef {
		p.v.Walker.StopWithInternalErr(errors.New("introspection root field is not set"))
	}

	return plan.FetchConfiguration{
		Input: p.configureInput(),
		DataSource: &Source{
			introspectionData: p.introspectionData,
		},
	}
}

func (p *Planner) ConfigureSubscription() plan.SubscriptionConfiguration {
	// the Introspection DataSourcePlanner doesn't have subscription
	return plan.SubscriptionConfiguration{}
}
