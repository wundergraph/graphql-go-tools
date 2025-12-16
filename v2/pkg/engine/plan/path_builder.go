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
	walker := astvisitor.NewWalkerWithID(48, "PathBuilderWalker")
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
		debugMessage("CreatePlanningPaths\n===================")
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
	p.visitor.populateMissingPaths()

	// walk ends in 2 cases:
	// - we have finished visiting document
	// - walker.Stop was called and visiting was halted

	if p.config.Debug.PrintPlanningPaths {
		fmt.Printf("\nPlanned paths on initial run #1:\n")
		p.printRevisitInfo()
		p.printPlanningPaths(1)
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
		p.visitor.populateMissingPaths()

		if p.config.Debug.PrintPlanningPaths {
			fmt.Printf("\nPlanned paths on run #%d:\n", i+1)
			p.printRevisitInfo()
			p.printPlanningPaths(i + 1)
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
		debugMessage("Paths after removing unnecessary fragment paths:")
		p.printPlanningPaths(i + 1)
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
	if p.visitor.shouldRevisit() {
		fmt.Println("\tshould revisit")
	}
	if p.visitor.hasMissingPaths() {
		fmt.Println("\thas missing paths")
	}
	if p.visitor.hasFieldsWaitingForDependency() {
		fmt.Println("\thas fields waiting for dependency")
	}

	p.printMissingPaths()
}

func (p *PathBuilder) printPlanningPaths(run int) {
	for i, planner := range p.visitor.planners {
		fmt.Printf("\n\tRun #%d. Planner ID %d\n", run, i)
		fmt.Printf("\t\tParent = %s\n", planner.ParentPath())
		ds := planner.DataSourceConfiguration()
		fmt.Printf("\t\tDatasource ID = %s, name = %s, hash = %d\n", ds.Id(), ds.Name(), ds.Hash())
		if len(planner.ObjectFetchConfiguration().dependsOnFetchIDs) > 0 {
			fmt.Printf("\t\tDepends on planner IDs: %v\n", planner.ObjectFetchConfiguration().dependsOnFetchIDs)
		}

		requiredFields := planner.RequiredFields()
		if requiredFields != nil && len(*requiredFields) > 0 {
			fmt.Println("\t\tRequired fields:")
			for _, field := range *requiredFields {
				if field.FieldName != "" {
					fmt.Printf("\t\t\trequired by %s: %s\n", field.FieldName, field)
					continue
				}
				fmt.Println("\t\t\tkey:", field)
			}
		}
		fmt.Println("\t\tPaths:")
		planner.ForEachPath(func(path *pathConfiguration) (shouldBreak bool) {
			fmt.Println(path.String())
			return false
		})
		fmt.Println()
	}
}

func (p *PathBuilder) printMissingPaths() {
	if p.visitor.hasMissingPaths() {
		fmt.Printf("\n\tMissing paths:\n")
		for path := range p.visitor.missingPathTracker {
			fmt.Printf("\t\t%v\n", path)
		}
	}
}
