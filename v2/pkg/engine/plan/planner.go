package plan

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type Planner struct {
	config Configuration

	planningWalker  *astvisitor.Walker
	planningVisitor *Visitor
	costVisitor     *StaticCostVisitor

	nodeSelectionBuilder *NodeSelectionBuilder
	planningPathBuilder  *PathBuilder

	prepareOperationWalker *astvisitor.Walker
}

// NewPlanner creates a new Planner from the Configuration.
//
// NOTE: All stateful DataSources should be initiated with the same context.Context object provided to the PlannerFactory.
// The context.Context object is used to determine the lifecycle of stateful DataSources.
// It's important to note that stateful DataSources must be closed when they are no longer being used.
//
// Stateful DataSources could be those that initiate a WebSocket connection to an origin, a database client, a streaming client, etc...
// To ensure that there are no memory leaks, it's therefore important to add a cancel func or timeout to the context.Context.
// At the time when the resolver and all operations should be garbage collected, ensure to first cancel or timeout the ctx object.
// If you don't cancel the context.Context, the goroutines will run indefinitely and there's no reference left to stop them.
func NewPlanner(config Configuration) (*Planner, error) {
	if config.Logger == nil {
		config.Logger = abstractlogger.Noop{}
	}

	entityInterfaceNames := make([]string, 0, 1)
	dsIDs := make(map[string]struct{}, len(config.DataSources))
	for _, ds := range config.DataSources {
		if _, ok := dsIDs[ds.Id()]; ok {
			return nil, fmt.Errorf("duplicate datasource id: %s", ds.Id())
		}
		dsIDs[ds.Id()] = struct{}{}

		entityInterfaceNames = append(entityInterfaceNames, ds.EntityInterfaceNames()...)
	}
	config.EntityInterfaceNames = entityInterfaceNames

	// prepare operation walker handles internal normalization for planner
	prepareOperationWalker := astvisitor.NewWalkerWithID(48, "PrepareOperationWalker")
	astnormalization.InlineFragmentAddOnType(&prepareOperationWalker)

	// planning

	planningWalker := astvisitor.NewWalkerWithID(48, "PlanningWalker")

	planningVisitor := &Visitor{
		Walker:                       &planningWalker,
		fieldConfigs:                 map[int]*FieldConfiguration{},
		disableResolveFieldPositions: config.DisableResolveFieldPositions,
	}

	p := &Planner{
		config:                 config,
		planningWalker:         &planningWalker,
		planningVisitor:        planningVisitor,
		prepareOperationWalker: &prepareOperationWalker,
	}

	return p, nil
}

type _opts struct {
	includeQueryPlanInResponse bool
}

type Opts func(*_opts)

func IncludeQueryPlanInResponse() Opts {
	return func(o *_opts) {
		o.includeQueryPlanInResponse = true
	}
}

func (p *Planner) Plan(operation, definition *ast.Document, operationName string, report *operationreport.Report, options ...Opts) (plan Plan) {

	var opts _opts
	for _, opt := range options {
		opt(&opts)
	}

	p.planningVisitor.includeQueryPlans = opts.includeQueryPlanInResponse

	p.selectOperation(operation, operationName, report)
	if report.HasErrors() {
		return
	}

	p.prepareOperation(operation, definition, report)
	if report.HasErrors() {
		return
	}

	// assign hash to each datasource
	for i := range p.config.DataSources {
		p.config.DataSources[i].Hash()
	}

	// create node selections
	if p.nodeSelectionBuilder == nil {
		p.nodeSelectionBuilder = NewNodeSelectionBuilder(&p.config)
	}
	p.nodeSelectionBuilder.SetOperationName(p.planningVisitor.OperationName)
	p.nodeSelectionBuilder.ResetSkipFieldRefs()

	selectionsConfig := p.nodeSelectionBuilder.SelectNodes(operation, definition, report)
	if report.HasErrors() {
		return nil
	}

	// create planning paths
	if p.planningPathBuilder == nil {
		p.planningPathBuilder = NewPathBuilder(&p.config)
	}
	p.planningPathBuilder.SetOperationName(p.planningVisitor.OperationName)
	p.planningPathBuilder.SetSelectionsConfig(selectionsConfig)

	plannersConfigurations := p.planningPathBuilder.CreatePlanningPaths(operation, definition, report)
	if report.HasErrors() {
		return nil
	}

	if p.config.Debug.PlanningVisitor {
		debugMessage("Planning Visitor\n================")
	}

	// configure planning visitor

	p.planningVisitor.planners = plannersConfigurations
	p.planningVisitor.Config = p.config
	p.planningVisitor.skipFieldsRefs = selectionsConfig.skipFieldsRefs
	p.planningVisitor.fieldRefDependsOnFieldRefs = selectionsConfig.fieldRefDependsOn
	p.planningVisitor.fieldDependencyKind = selectionsConfig.fieldDependencyKind
	p.planningVisitor.fieldRefDependants = inverseMap(selectionsConfig.fieldRefDependsOn)

	p.planningWalker.ResetVisitors()
	p.planningWalker.SetVisitorFilter(p.planningVisitor)
	p.planningWalker.RegisterDocumentVisitor(p.planningVisitor)
	p.planningWalker.RegisterEnterOperationVisitor(p.planningVisitor)
	p.planningWalker.RegisterFieldVisitor(p.planningVisitor)
	p.planningWalker.RegisterSelectionSetVisitor(p.planningVisitor)
	p.planningWalker.RegisterEnterDirectiveVisitor(p.planningVisitor)
	p.planningWalker.RegisterInlineFragmentVisitor(p.planningVisitor)

	// Register cost visitor on the same walker (will be invoked after planningVisitor hooks).
	// We have to register it last in the walker, as it depends on the fieldPlanners field of the
	// visitor. That field is populated in the AllowVisitor callback. Walker calls Enter* callbacks
	// in the order they were registered, and Leave* callbacks in the reverse order.
	if p.config.ComputeCosts {
		p.costVisitor = NewStaticCostVisitor(p.planningWalker, operation, definition)
		p.costVisitor.planners = plannersConfigurations
		p.costVisitor.fieldPlanners = &p.planningVisitor.fieldPlanners
		p.costVisitor.operationDefinition = &p.planningVisitor.operationDefinitionRef

		p.planningWalker.RegisterEnterFieldVisitor(p.costVisitor)
		p.planningWalker.RegisterLeaveFieldVisitor(p.costVisitor)
	}

	for key := range p.planningVisitor.planners {
		if p.config.MinifySubgraphOperations {
			if dataSourceWithMinify, ok := p.planningVisitor.planners[key].Planner().(SubgraphRequestMinifier); ok {
				dataSourceWithMinify.EnableSubgraphRequestMinifier()
			}
		}
		if opts.includeQueryPlanInResponse {
			if plannerWithQueryPlan, ok := p.planningVisitor.planners[key].Planner().(QueryPlanProvider); ok {
				plannerWithQueryPlan.IncludeQueryPlanInFetchConfiguration()
			}
		}
		if plannerWithId, ok := p.planningVisitor.planners[key].Planner().(Identifyable); ok {
			plannerWithId.SetID(key)
		}
		if plannerWithDebug, ok := p.planningVisitor.planners[key].Debugger(); ok {
			if p.config.Debug.DatasourceVisitor {
				plannerWithDebug.EnableDebug()
			}

			if p.config.Debug.PrintQueryPlans {
				plannerWithDebug.EnableDebugQueryPlanLogging()
			}
		}
		err := p.planningVisitor.planners[key].Register(p.planningVisitor)
		if err != nil {
			report.AddInternalError(err)
			return
		}
	}

	// create a raw execution plan
	p.planningWalker.Walk(operation, definition, report)
	if report.HasErrors() {
		return
	}

	if p.config.ComputeCosts {
		costCalc := NewCostCalculator()
		costCalc.tree = p.costVisitor.finalCostTree()
		p.planningVisitor.plan.SetCostCalculator(costCalc)
	}

	return p.planningVisitor.plan
}

func (p *Planner) selectOperation(operation *ast.Document, operationName string, report *operationreport.Report) {

	numOfOperations := operation.NumOfOperationDefinitions()
	operationName = strings.TrimSpace(operationName)
	if len(operationName) == 0 && numOfOperations > 1 {
		report.AddExternalError(operationreport.ErrRequiredOperationNameIsMissing())
		return
	}

	if len(operationName) == 0 && numOfOperations == 1 {
		operationName = operation.OperationDefinitionNameString(0)
	}

	if !operation.OperationNameExists(operationName) {
		report.AddExternalError(operationreport.ErrOperationWithProvidedOperationNameNotFound(operationName))
		return
	}

	p.planningVisitor.OperationName = operationName
}

func (p *Planner) prepareOperation(operation, definition *ast.Document, report *operationreport.Report) {
	p.prepareOperationWalker.Walk(operation, definition, report)
}

func inverseMap(m map[int][]int) map[int][]int {
	inverse := make(map[int][]int)
	for k, v := range m {
		for _, v2 := range v {
			inverse[v2] = append(inverse[v2], k)
		}
	}
	// Normalize ordering for deterministic plans/tests
	for key := range inverse {
		sort.Ints(inverse[key])
	}
	return inverse
}

func debugMessage(msg string) {
	fmt.Printf("\n\n%s\n\n", msg)
}
