package plan

import (
	"context"
	"fmt"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type Planner struct {
	config               Configuration
	configurationWalker  *astvisitor.Walker
	configurationVisitor *configurationVisitor
	planningWalker       *astvisitor.Walker
	planningVisitor      *Visitor
}

// NewPlanner creates a new Planner from the Configuration and a ctx object
// The context.Context object is used to determine the lifecycle of stateful DataSources
// It's important to note that stateful DataSources must be closed when they are no longer being used
// Stateful DataSources could be those that initiate a WebSocket connection to an origin, a database client, a streaming client, etc...
// All DataSources are initiated with the same context.Context object provided to the Planner.
// To ensure that there are no memory leaks, it's therefore important to add a cancel func or timeout to the context.Context
// At the time when the resolver and all operations should be garbage collected, ensure to first cancel or timeout the ctx object
// If you don't cancel the context.Context, the goroutines will run indefinitely and there's no reference left to stop them
func NewPlanner(ctx context.Context, config Configuration) *Planner {
	// configuration

	configurationWalker := astvisitor.NewWalker(48)
	configVisitor := &configurationVisitor{
		walker: &configurationWalker,
		ctx:    ctx,
	}

	configurationWalker.RegisterEnterDocumentVisitor(configVisitor)
	configurationWalker.RegisterFieldVisitor(configVisitor)
	configurationWalker.RegisterEnterOperationVisitor(configVisitor)
	configurationWalker.RegisterSelectionSetVisitor(configVisitor)

	// planning

	planningWalker := astvisitor.NewWalker(48)
	planningVisitor := &Visitor{
		Walker:                       &planningWalker,
		fieldConfigs:                 map[int]*FieldConfiguration{},
		disableResolveFieldPositions: config.DisableResolveFieldPositions,
	}

	p := &Planner{
		config:               config,
		configurationWalker:  &configurationWalker,
		configurationVisitor: configVisitor,
		planningWalker:       &planningWalker,
		planningVisitor:      planningVisitor,
	}

	return p
}

func (p *Planner) SetConfig(config Configuration) {
	p.config = config
}

func (p *Planner) SetDebugConfig(config DebugConfiguration) {
	p.config.Debug = config
}

func (p *Planner) Plan(operation, definition *ast.Document, operationName string, report *operationreport.Report) (plan Plan) {
	p.selectOperation(operation, operationName, report)
	if report.HasErrors() {
		return
	}

	// assign hash to each datasource
	for i := range p.config.DataSources {
		p.config.DataSources[i].Hash()
	}

	p.findPlanningPaths(operation, definition, report)
	if report.HasErrors() {
		return nil
	}

	if p.config.Debug.PlanningVisitor {
		p.debugMessage("Planning visitor:")
	}

	// configure planning visitor

	p.planningVisitor.planners = p.configurationVisitor.planners
	p.planningVisitor.Config = p.config
	p.planningVisitor.fetchConfigurations = p.configurationVisitor.fetches
	p.planningVisitor.skipFieldsRefs = p.configurationVisitor.skipFieldsRefs

	p.planningWalker.ResetVisitors()
	p.planningWalker.SetVisitorFilter(p.planningVisitor)
	p.planningWalker.RegisterDocumentVisitor(p.planningVisitor)
	p.planningWalker.RegisterEnterOperationVisitor(p.planningVisitor)
	p.planningWalker.RegisterFieldVisitor(p.planningVisitor)
	p.planningWalker.RegisterSelectionSetVisitor(p.planningVisitor)
	p.planningWalker.RegisterEnterDirectiveVisitor(p.planningVisitor)
	p.planningWalker.RegisterInlineFragmentVisitor(p.planningVisitor)

	for key := range p.planningVisitor.planners {
		dataSourceConfig := p.planningVisitor.planners[key].dataSourceConfiguration
		dataSourcePlannerConfig := DataSourcePlannerConfiguration{
			RequiredFields: p.planningVisitor.planners[key].requiredFields,
			ProvidedFields: p.planningVisitor.planners[key].providedFields,
			ParentPath:     p.planningVisitor.planners[key].parentPath,
			PathType:       p.planningVisitor.planners[key].parentPathType,
			IsNested:       p.planningVisitor.planners[key].isNestedPlanner(),
		}

		if plannerWithId, ok := p.planningVisitor.planners[key].planner.(astvisitor.VisitorIdentifier); ok {
			plannerWithId.SetID(key + 1)
		}
		if plannerWithDebug, ok := p.planningVisitor.planners[key].planner.(DataSourceDebugger); ok {
			if p.config.Debug.DatasourceVisitor {
				plannerWithDebug.EnableDebug()
			}

			if p.config.Debug.PrintQueryPlans {
				plannerWithDebug.EnableQueryPlanLogging()
			}
		}

		err := p.planningVisitor.planners[key].planner.Register(p.planningVisitor, dataSourceConfig, dataSourcePlannerConfig)
		if err != nil {
			report.AddInternalError(err)
			return
		}
	}

	// process the plan

	p.planningWalker.Walk(operation, definition, report)
	if report.HasErrors() {
		return
	}

	return p.planningVisitor.plan
}

func (p *Planner) findPlanningPaths(operation, definition *ast.Document, report *operationreport.Report) {
	dsFilter := NewDataSourceFilter(operation, definition, report)

	if p.config.Debug.PrintOperationWithRequiredFields {
		p.debugMessage("Initial operation:")
		p.printOperation(operation)
	}

	p.configurationVisitor.debug = p.config.Debug.ConfigurationVisitor

	// set initial suggestions and used data sources
	p.configurationVisitor.dataSources, p.configurationVisitor.nodeSuggestions =
		dsFilter.FilterDataSources(p.config.DataSources, nil)
	if report.HasErrors() {
		return
	}

	if p.config.Debug.PrintNodeSuggestions {
		p.debugMessage("Initial node suggestions:")
		for i := range p.configurationVisitor.nodeSuggestions {
			fmt.Println(p.configurationVisitor.nodeSuggestions[i].String())
		}
	}

	p.configurationVisitor.secondaryRun = false
	p.configurationWalker.Walk(operation, definition, report)
	if report.HasErrors() {
		return
	}

	if p.config.Debug.PrintOperationWithRequiredFields {
		p.debugMessage("Operation after initial run:")
		p.printOperation(operation)
	}

	if p.config.Debug.PrintPlanningPaths {
		p.printPlanningPaths()
	}

	i := 1
	// secondary runs to add path for the new required fields
	for p.configurationVisitor.hasNewFields || p.configurationVisitor.hasMissingPaths() {
		p.configurationVisitor.secondaryRun = true

		// update suggestions for the new required fields
		p.configurationVisitor.dataSources, p.configurationVisitor.nodeSuggestions =
			dsFilter.FilterDataSources(p.config.DataSources, p.configurationVisitor.nodeSuggestions)
		if report.HasErrors() {
			return
		}

		if p.config.Debug.PrintNodeSuggestions {
			p.debugMessage("Recalculated node suggestions:")
			for i := range p.configurationVisitor.nodeSuggestions {
				fmt.Println(p.configurationVisitor.nodeSuggestions[i].String())
			}
		}

		p.configurationWalker.Walk(operation, definition, report)
		if report.HasErrors() {
			return
		}

		if p.config.Debug.PrintOperationWithRequiredFields {
			p.debugMessage(fmt.Sprintf("After run #%d. Operation with new required fields:", i))
			p.debugMessage(fmt.Sprintf("Has new fields: %v", p.configurationVisitor.hasNewFields))
			p.printOperation(operation)
		}

		if p.config.Debug.PrintPlanningPaths {
			p.debugMessage(fmt.Sprintf("After run #%d. Planning paths", i))
			if p.configurationVisitor.hasMissingPaths() {
				p.debugMessage("Missing paths:")
				for path := range p.configurationVisitor.missingPathTracker {
					fmt.Println(path)
				}
			}
			p.printPlanningPaths()
		}
		i++

		if i > 100 {
			report.AddInternalError(fmt.Errorf("bad datasource configuration - could not plan the operation"))
			return
		}
	}

	// remove unnecessary fragment paths
	hasRemovedPaths := p.removeUnnecessaryFragmentPaths()
	if hasRemovedPaths && p.config.Debug.PrintPlanningPaths {
		p.debugMessage("After removing unnecessary fragment paths")
		p.printPlanningPaths()
	}
}

func (p *Planner) removeUnnecessaryFragmentPaths() (hasRemovedPaths bool) {
	// We add fragment paths on enter selection set of fragments in configurationVisitor
	// It could happen that datasource has a root node for the given fragment type,
	// but we do not select any fields from this fragment
	// So we need to remove all fragment paths that are not prefixes of any other path

	for _, planner := range p.configurationVisitor.planners {
		fragmentPaths := planner.fragmentPaths()
		for _, path := range fragmentPaths {
			if !planner.hasPathPrefix(path) {
				planner.removePath(path)
				hasRemovedPaths = true
			}
		}
	}
	return
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

	p.configurationVisitor.operationName = operationName
	p.planningVisitor.OperationName = operationName
}

func (p *Planner) printOperation(operation *ast.Document) {
	pp, _ := astprinter.PrintStringIndentDebug(operation, nil, "  ")
	fmt.Println(pp)
}

func (p *Planner) printPlanningPaths() {
	p.debugMessage("Planning paths:")
	for i, planner := range p.configurationVisitor.planners {
		fmt.Println("Paths for planner", i+1)
		fmt.Println("Planner parent path", planner.parentPath)
		for _, path := range planner.paths {
			fmt.Println(path.String())
		}
	}
}

func (p *Planner) debugMessage(msg string) {
	fmt.Printf("\n\n%s\n\n", msg)
}
