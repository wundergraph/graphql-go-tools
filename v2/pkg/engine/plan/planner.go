package plan

import (
	"fmt"
	"strings"

	"github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
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

	prepareOperationWalker *astvisitor.Walker
}

// NewPlanner creates a new Planner from the Configuration
// NOTE: All stateful DataSources should be initiated with the same context.Context object provided to the PlannerFactory.
// The context.Context object is used to determine the lifecycle of stateful DataSources
// It's important to note that stateful DataSources must be closed when they are no longer being used
// Stateful DataSources could be those that initiate a WebSocket connection to an origin, a database client, a streaming client, etc...
// To ensure that there are no memory leaks, it's therefore important to add a cancel func or timeout to the context.Context
// At the time when the resolver and all operations should be garbage collected, ensure to first cancel or timeout the ctx object
// If you don't cancel the context.Context, the goroutines will run indefinitely and there's no reference left to stop them
func NewPlanner(config Configuration) (*Planner, error) {
	if config.Logger == nil {
		config.Logger = abstractlogger.Noop{}
	}

	dsIDs := make(map[string]struct{}, len(config.DataSources))
	for _, ds := range config.DataSources {
		if _, ok := dsIDs[ds.Id()]; ok {
			return nil, fmt.Errorf("duplicate datasource id: %s", ds.Id())
		}
		dsIDs[ds.Id()] = struct{}{}
	}

	// prepare operation walker handles internal normalization for planner
	prepareOperationWalker := astvisitor.NewWalker(48)
	astnormalization.InlineFragmentAddOnType(&prepareOperationWalker)

	// configuration
	configurationWalker := astvisitor.NewWalker(48)
	configVisitor := &configurationVisitor{
		walker:              &configurationWalker,
		fieldConfigurations: config.Fields,
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
		config:                 config,
		configurationWalker:    &configurationWalker,
		configurationVisitor:   configVisitor,
		planningWalker:         &planningWalker,
		planningVisitor:        planningVisitor,
		prepareOperationWalker: &prepareOperationWalker,
	}

	return p, nil
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

	p.prepareOperation(operation, definition, report)
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
		if plannerWithId, ok := p.planningVisitor.planners[key].Planner().(astvisitor.VisitorIdentifier); ok {
			plannerWithId.SetID(key + 1)
		}
		if plannerWithDebug, ok := p.planningVisitor.planners[key].Debugger(); ok {
			if p.config.Debug.DatasourceVisitor {
				plannerWithDebug.EnableDebug()
			}

			if p.config.Debug.PrintQueryPlans {
				plannerWithDebug.EnableQueryPlanLogging()
			}
		}

		err := p.planningVisitor.planners[key].Register(p.planningVisitor)
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

	if p.config.Debug.EnableNodeSuggestionsSelectionReasons {
		dsFilter.EnableSelectionReasons()
	}

	if p.config.Debug.PrintOperationTransformations {
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
		p.configurationVisitor.nodeSuggestions.printNodes("\n\nInitial node suggestions:\n\n")
	}

	p.configurationVisitor.secondaryRun = false
	p.configurationWalker.Walk(operation, definition, report)
	if report.HasErrors() {
		return
	}

	if p.config.Debug.PrintOperationTransformations {
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

		if p.configurationVisitor.hasNewFields {
			// update suggestions for the new required fields
			p.configurationVisitor.dataSources, p.configurationVisitor.nodeSuggestions =
				dsFilter.FilterDataSources(p.config.DataSources, p.configurationVisitor.nodeSuggestions, p.configurationVisitor.nodeSuggestionHints...)
			if report.HasErrors() {
				return
			}

			if p.config.Debug.PrintNodeSuggestions {
				p.configurationVisitor.nodeSuggestions.printNodes("\n\nRecalculated node suggestions:\n\n")
			}
		}

		p.configurationWalker.Walk(operation, definition, report)
		if report.HasErrors() {
			return
		}

		if p.config.Debug.PrintOperationTransformations {
			p.debugMessage(fmt.Sprintf("After run #%d. Operation with new required fields:", i))
			p.debugMessage(fmt.Sprintf("Has new fields: %v", p.configurationVisitor.hasNewFields))
			p.printOperation(operation)
		}

		if p.config.Debug.PrintPlanningPaths {
			p.debugMessage(fmt.Sprintf("After run #%d. Planning paths", i))
			p.printPlanningPaths()
		}
		i++

		if i > 100 {
			missingPaths := make([]string, 0, len(p.configurationVisitor.missingPathTracker))
			for path := range p.configurationVisitor.missingPathTracker {
				missingPaths = append(missingPaths, path)
			}

			report.AddInternalError(fmt.Errorf("bad datasource configuration - could not plan the operation. missing path: %v", missingPaths))
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
		if planner.RemoveLeafFragmentPaths() {
			hasRemovedPaths = true
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

func (p *Planner) prepareOperation(operation, definition *ast.Document, report *operationreport.Report) {
	p.prepareOperationWalker.Walk(operation, definition, report)
}

func (p *Planner) printOperation(operation *ast.Document) {
	var pp string

	if p.config.Debug.PrintOperationEnableASTRefs {
		pp, _ = astprinter.PrintStringIndentDebug(operation, nil, "  ")
	} else {
		pp, _ = astprinter.PrintStringIndent(operation, nil, "  ")
	}

	fmt.Println(pp)
}

func (p *Planner) printPlanningPaths() {
	p.debugMessage("Planning paths:")
	for i, planner := range p.configurationVisitor.planners {
		fmt.Println("Paths for planner", i+1)
		fmt.Println("Planner parent path", planner.ParentPath())
		planner.ForEachPath(func(path *pathConfiguration) (shouldBreak bool) {
			fmt.Println(path.String())
			return false
		})
	}

	if p.configurationVisitor.hasMissingPaths() {
		p.debugMessage("Missing paths:")
		for _, path := range p.configurationVisitor.missingPathTracker {
			fmt.Println(path.String())
		}
	}
}

func (p *Planner) debugMessage(msg string) {
	fmt.Printf("\n\n%s\n\n", msg)
}
