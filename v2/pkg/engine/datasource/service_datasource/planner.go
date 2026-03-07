package service_datasource

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

const (
	serviceFieldName = "__service"
)

// Configuration is the configuration for the service datasource.
type Configuration struct {
	SourceType string
}

// Planner is the planner for the __service field.
type Planner[T Configuration] struct {
	id            int
	service       *Service
	v             *plan.Visitor
	rootField     int
	rootFieldPath string
}

// SetID implements the DataSourcePlanner interface.
func (p *Planner[T]) SetID(id int) {
	p.id = id
}

// ID implements the DataSourcePlanner interface.
func (p *Planner[T]) ID() (id int) {
	return p.id
}

// Register implements the DataSourcePlanner interface.
func (p *Planner[T]) Register(visitor *plan.Visitor, dataSourceConfiguration plan.DataSourceConfiguration[T], dataSourcePlannerConfiguration plan.DataSourcePlannerConfiguration) error {
	p.v = visitor
	p.rootField = ast.InvalidRef
	visitor.Walker.RegisterEnterFieldVisitor(p)
	return nil
}

// DownstreamResponseFieldAlias implements the DataSourcePlanner interface.
func (p *Planner[T]) DownstreamResponseFieldAlias(_ int) (alias string, exists bool) {
	return
}

// EnterField is called when entering a field.
func (p *Planner[T]) EnterField(ref int) {
	fieldName := p.v.Operation.FieldNameString(ref)
	fieldAliasOrName := p.v.Operation.FieldAliasOrNameString(ref)
	if fieldName == serviceFieldName {
		p.rootField = ref
		p.rootFieldPath = fieldAliasOrName
	}
}

// ConfigureFetch implements the DataSourcePlanner interface.
func (p *Planner[T]) ConfigureFetch() resolve.FetchConfiguration {
	if p.rootField == ast.InvalidRef {
		return resolve.FetchConfiguration{}
	}

	postProcessing := resolve.PostProcessingConfiguration{
		MergePath: []string{p.rootFieldPath},
	}

	return resolve.FetchConfiguration{
		Input:          `{}`,
		DataSource:     NewSource(p.service),
		PostProcessing: postProcessing,
	}
}

// ConfigureSubscription implements the DataSourcePlanner interface.
func (p *Planner[T]) ConfigureSubscription() plan.SubscriptionConfiguration {
	return plan.SubscriptionConfiguration{}
}
