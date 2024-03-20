package introspection_datasource

import (
	"errors"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/introspection"
)

type Configuration struct {
	SourceType string
}

type Planner[T Configuration] struct {
	introspectionData *introspection.Data
	v                 *plan.Visitor
	rootField         int
	rootFieldName     string
	rootFielPath      string
	isArrayItem       bool
}

func (p *Planner[T]) UpstreamSchema(dataSourceConfig plan.DataSourceConfiguration[T]) (*ast.Document, bool) {
	return nil, false
}

func (p *Planner[T]) Register(visitor *plan.Visitor, dataSourceConfiguration plan.DataSourceConfiguration[T], dataSourcePlannerConfiguration plan.DataSourcePlannerConfiguration) error {
	p.v = visitor
	p.rootField = ast.InvalidRef
	p.isArrayItem = dataSourcePlannerConfiguration.PathType == plan.PlannerPathArrayItem
	visitor.Walker.RegisterEnterFieldVisitor(p)
	return nil
}

func (p *Planner[T]) DownstreamResponseFieldAlias(_ int) (alias string, exists bool) {
	// the Introspection DataSourcePlanner doesn't rewrite upstream fields: skip
	return
}

func (p *Planner[T]) DataSourcePlanningBehavior() plan.DataSourcePlanningBehavior {
	return plan.DataSourcePlanningBehavior{
		MergeAliasedRootNodes:      false,
		OverrideFieldPathFromAlias: true,
	}
}

func (p *Planner[T]) EnterField(ref int) {
	fieldName := p.v.Operation.FieldNameString(ref)
	fieldAliasOrName := p.v.Operation.FieldAliasOrNameString(ref)
	switch fieldName {
	case typeFieldName, fieldsFieldName, enumValuesFieldName, schemaFieldName:
		p.rootField = ref
		p.rootFieldName = fieldName
		p.rootFielPath = fieldAliasOrName
	}
}

func (p *Planner[T]) configureInput() string {
	return buildInput(p.rootFieldName)
}

func (p *Planner[T]) ConfigureFetch() resolve.FetchConfiguration {
	if p.rootField == ast.InvalidRef {
		p.v.Walker.StopWithInternalErr(errors.New("introspection root field is not set"))
	}

	postProcessing := resolve.PostProcessingConfiguration{
		MergePath: []string{p.rootFielPath},
	}

	requiresParallelListItemFetch := false
	switch p.rootFieldName {
	case fieldsFieldName, enumValuesFieldName:
		requiresParallelListItemFetch = p.isArrayItem
	}

	return resolve.FetchConfiguration{
		Input:                         p.configureInput(),
		RequiresParallelListItemFetch: requiresParallelListItemFetch,
		DataSource: &Source{
			introspectionData: p.introspectionData,
		},
		PostProcessing: postProcessing,
	}
}

func (p *Planner[T]) ConfigureSubscription() plan.SubscriptionConfiguration {
	// the Introspection DataSourcePlanner doesn't have subscription
	return plan.SubscriptionConfiguration{}
}
