package grpc_datasource

import (
	"github.com/wundergraph/graphql-go-tools/pkg/engine/plan"
)

type Planner struct {
	v         *plan.Visitor
	rootField int
	config    Configuration
}

func (p *Planner) Register(visitor *plan.Visitor, _ plan.DataSourceConfiguration, _ bool) error {
	p.v = visitor
	visitor.Walker.RegisterEnterFieldVisitor(p)
	return nil
}

func (p *Planner) DownstreamResponseFieldAlias(_ int) (alias string, exists bool) {
	return
}

func (p *Planner) DataSourcePlanningBehavior() plan.DataSourcePlanningBehavior {
	return plan.DataSourcePlanningBehavior{
		MergeAliasedRootNodes:      false,
		OverrideFieldPathFromAlias: false,
	}
}

func (p *Planner) EnterField(ref int) {
	p.rootField = ref
}

func (p *Planner) configureInput() string {
	return ""
}

func (p *Planner) ConfigureFetch() plan.FetchConfiguration {
	return plan.FetchConfiguration{
		Input: p.configureInput(),
		DataSource: &Source{
			config: p.config,
		},
	}
}

func (p *Planner) ConfigureSubscription() plan.SubscriptionConfiguration {

	return plan.SubscriptionConfiguration{}
}
