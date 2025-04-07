package plan

import (
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type PathBuilder struct {
	config           *Configuration
	selectionsConfig *NodeSelectionResult

	walker  *astvisitor.Walker
	visitor *pathBuilderVisitor
}

func NewPathBuilder(config *Configuration) *PathBuilder {
	walker := astvisitor.NewWalker(48)
	visitor := &pathBuilderVisitor{
		walker:              &walker,
		fieldConfigurations: config.Fields,
	}

	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterFieldVisitor(visitor)
	walker.RegisterEnterOperationVisitor(visitor)
	walker.RegisterSelectionSetVisitor(visitor)

	return &PathBuilder{
		config:  config,
		walker:  &walker,
		visitor: visitor,
	}
}

func (p *PathBuilder) SetSelectionsConfig(selectionsConfig *NodeSelectionResult) {
	p.selectionsConfig = selectionsConfig
}

func (p *PathBuilder) SetOperationName(name string) {
	p.visitor.operationName = name
}

func (p *PathBuilder) CreatePlanningPaths(operation, definition *ast.Document, report *operationreport.Report) []PlannerConfiguration {
	if p.config.Debug.PrintPlanningPaths {
		debugMessage("Create planning paths")
	}

	p.visitor.plannerConfiguration = p.config

	// set initial suggestions and used data sources
	p.visitor.dataSources, p.visitor.nodeSuggestions =
		p.selectionsConfig.dataSources, p.selectionsConfig.nodeSuggestions

	// set fields dependencies information
	p.visitor.fieldDependsOn, p.visitor.fieldRequirementsConfigs =
		p.selectionsConfig.fieldDependsOn, p.selectionsConfig.fieldRequirementsConfigs

	p.visitor.secondaryRun = false
	p.walker.Walk(operation, definition, report)
	if report.HasErrors() {
		return nil
	}
	// we have to populate missing paths after the walk
	p.visitor.populateMissingPahts()

	// walk ends in 2 cases:
	// - we have finished visiting document
	// - walker.Stop was called and visiting was halted

	if p.config.Debug.PrintPlanningPaths {
		debugMessage("Planning paths after initial run")
		p.printRevisitInfo()
		p.printPlanningPaths()
	}

	i := 1
	// secondary runs to add path for the new required fields
	for p.visitor.shouldRevisit() {
		p.visitor.secondaryRun = true

		p.walker.Walk(operation, definition, report)
		if report.HasErrors() {
			return nil
		}
		// we have to populate missing paths after the walk
		p.visitor.populateMissingPahts()

		if p.config.Debug.PrintPlanningPaths {
			debugMessage(fmt.Sprintf("Create planning paths run #%d", i))
		}

		if p.config.Debug.PrintPlanningPaths {
			p.printRevisitInfo()
			p.printPlanningPaths()
		}

		i++

		if i > 100 {
			missingPaths := make([]string, 0, len(p.visitor.missingPathTracker))
			for path := range p.visitor.missingPathTracker {
				missingPaths = append(missingPaths, path)
			}

			report.AddInternalError(fmt.Errorf("failed to obtain planning paths: %w", newFailedToCreatePlanningPathsError(
				missingPaths,
				p.visitor.hasFieldsWaitingForDependency(),
			)))
			return nil
		}
	}

	// remove unnecessary fragment paths
	hasRemovedPaths := p.removeUnnecessaryFragmentPaths()
	if hasRemovedPaths && p.config.Debug.PrintPlanningPaths {
		debugMessage("After removing unnecessary fragment paths")
		p.printPlanningPaths()
	}

	return p.visitor.planners
}

func (p *PathBuilder) removeUnnecessaryFragmentPaths() (hasRemovedPaths bool) {
	// We add fragment paths on enter selection set of fragments in pathBuilderVisitor
	// It could happen that datasource has a root node for the given fragment type,
	// but we do not select any fields from this fragment
	// So we need to remove all fragment paths that are not prefixes of any other path

	for _, planner := range p.visitor.planners {
		if planner.RemoveLeafFragmentPaths() {
			hasRemovedPaths = true
		}
	}
	return
}

func (p *PathBuilder) printRevisitInfo() {
	fmt.Println("Should revisit:", p.visitor.shouldRevisit())
	fmt.Println("Has missing paths:", p.visitor.hasMissingPaths())
	fmt.Println("Has fields waiting for dependency:", p.visitor.hasFieldsWaitingForDependency())

	p.printMissingPaths()
}

func (p *PathBuilder) printPlanningPaths() {
	debugMessage("\n\nPlanning paths:\n\n")
	for i, planner := range p.visitor.planners {
		fmt.Printf("\nPlanner id: %d\n", i)
		fmt.Printf("Parent path: %s\n", planner.ParentPath())
		ds := planner.DataSourceConfiguration()
		fmt.Printf("Datasource id: %s name: %s hash: %d\n", ds.Id(), ds.Name(), ds.Hash())
		fmt.Printf("Depends on planner ids: %v\n", planner.ObjectFetchConfiguration().dependsOnFetchIDs)

		requiredFields := planner.RequiredFields()
		if requiredFields != nil && len(*requiredFields) > 0 {
			fmt.Println("Required fields:")
			for _, field := range *requiredFields {
				fmt.Println(field)
			}
		}
		fmt.Println("Paths:")
		planner.ForEachPath(func(path *pathConfiguration) (shouldBreak bool) {
			fmt.Println(path.String())
			return false
		})
		fmt.Println()
	}
}

func (p *PathBuilder) printMissingPaths() {
	if p.visitor.hasMissingPaths() {
		debugMessage("Missing paths:")
		for path := range p.visitor.missingPathTracker {
			fmt.Println(path)
		}
	}
}
