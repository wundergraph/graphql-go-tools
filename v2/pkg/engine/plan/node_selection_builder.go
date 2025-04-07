package plan

import (
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type NodeSelectionBuilder struct {
	config *Configuration

	nodeResolvableWalker  *astvisitor.Walker
	nodeResolvableVisitor *nodesResolvableVisitor
	nodeSelectionsWalker  *astvisitor.Walker
	nodeSelectionsVisitor *nodeSelectionVisitor
}

type NodeSelectionResult struct {
	dataSources              []DataSource                                     // data sources configurations, which used by the current operation
	nodeSuggestions          *NodeSuggestions                                 // nodeSuggestions holds information about suggested data sources for each field
	fieldDependsOn           map[fieldIndexKey][]int                          // fieldDependsOn is a map[fieldIndexKey][]fieldRef - holds list of field refs which are required by a field ref, e.g. field should be planned only after required fields were planned
	fieldRequirementsConfigs map[fieldIndexKey][]FederationFieldConfiguration // fieldRequirementsConfigs is a map[fieldIndexKey]FederationFieldConfiguration - holds a list of required configuratuibs for a field ref to later built representation variables
	skipFieldsRefs           []int                                            // skipFieldsRefs holds required field refs added by planner and should not be added to user response
}

func NewNodeSelectionBuilder(config *Configuration, operationName string) *NodeSelectionBuilder {
	nodeSelectionsWalker := astvisitor.NewWalker(48)
	nodeSelectionVisitor := &nodeSelectionVisitor{
		walker:        &nodeSelectionsWalker,
		operationName: operationName, // TODO: check should not be needed
	}

	nodeSelectionsWalker.RegisterEnterDocumentVisitor(nodeSelectionVisitor)
	nodeSelectionsWalker.RegisterFieldVisitor(nodeSelectionVisitor)
	nodeSelectionsWalker.RegisterEnterOperationVisitor(nodeSelectionVisitor)
	nodeSelectionsWalker.RegisterSelectionSetVisitor(nodeSelectionVisitor)

	nodeResolvableWalker := astvisitor.NewWalker(32)
	nodeResolvableVisitor := &nodesResolvableVisitor{
		walker: &nodeResolvableWalker,
	}
	nodeResolvableWalker.RegisterEnterDocumentVisitor(nodeResolvableVisitor)
	nodeResolvableWalker.RegisterEnterFieldVisitor(nodeResolvableVisitor)

	return &NodeSelectionBuilder{
		config:                config,
		nodeSelectionsWalker:  &nodeSelectionsWalker,
		nodeSelectionsVisitor: nodeSelectionVisitor,
		nodeResolvableWalker:  &nodeResolvableWalker,
		nodeResolvableVisitor: nodeResolvableVisitor,
	}
}

func (p *NodeSelectionBuilder) SelectNodes(operation, definition *ast.Document, report *operationreport.Report) (out *NodeSelectionResult) {
	dsFilter := NewDataSourceFilter(operation, definition, report)

	if p.config.Debug.PrintNodeSuggestions {
		dsFilter.EnableSelectionReasons()
	}

	if p.config.Debug.PrintOperationTransformations {
		debugMessage("Initial operation:")
		p.printOperation(operation)
	}

	p.nodeSelectionsVisitor.debug = p.config.Debug

	// set initial suggestions and used data sources
	p.nodeSelectionsVisitor.dataSources, p.nodeSelectionsVisitor.nodeSuggestions =
		dsFilter.FilterDataSources(p.config.DataSources, nil, nil, nil)
	if report.HasErrors() {
		return
	}

	if p.config.Debug.PrintNodeSuggestions {
		p.nodeSelectionsVisitor.nodeSuggestions.printNodesWithFilter("\nInitial node suggestions:\n", p.config.Debug.PrintNodeSuggestionsFilterNotSelected)
	}

	p.nodeSelectionsVisitor.secondaryRun = false
	p.nodeSelectionsWalker.Walk(operation, definition, report)
	if report.HasErrors() {
		return
	}

	if p.config.Debug.PrintOperationTransformations {
		debugMessage("Select nodes initial run - operation:")
		p.printOperation(operation)
	}

	i := 1
	// secondary runs to add path for the new required fields
	for p.nodeSelectionsVisitor.shouldRevisit() {

		// when we have rewritten a field old node suggestion are not make sense anymore
		// so we are removing child nodes of the rewritten fields
		for _, fieldRef := range p.nodeSelectionsVisitor.rewrittenFieldRefs {
			p.nodeSelectionsVisitor.nodeSuggestions.RemoveTreeNodeChilds(fieldRef)
		}

		p.nodeSelectionsVisitor.secondaryRun = true

		if p.nodeSelectionsVisitor.hasNewFields {
			// update suggestions for the new required fields
			p.nodeSelectionsVisitor.dataSources, p.nodeSelectionsVisitor.nodeSuggestions =
				dsFilter.FilterDataSources(p.config.DataSources, p.nodeSelectionsVisitor.nodeSuggestions, p.nodeSelectionsVisitor.fieldLandedTo, p.nodeSelectionsVisitor.fieldRefDependsOn)
			if report.HasErrors() {
				return
			}
		}

		if p.config.Debug.PrintOperationTransformations || p.config.Debug.PrintNodeSuggestions {
			debugMessage(fmt.Sprintf("Select nodes run #%d", i))
		}

		if p.config.Debug.PrintNodeSuggestions {
			p.nodeSelectionsVisitor.nodeSuggestions.printNodesWithFilter("\nRecalculated node suggestions:\n", p.config.Debug.PrintNodeSuggestionsFilterNotSelected)
		}

		p.nodeSelectionsWalker.Walk(operation, definition, report)
		if report.HasErrors() {
			return
		}

		if p.config.Debug.PrintOperationTransformations {
			debugMessage("Operation with new required fields:")
			debugMessage(fmt.Sprintf("Has new fields: %v", p.nodeSelectionsVisitor.hasNewFields))
			p.printOperation(operation)
		}

		i++

		if resolvableReport := p.isResolvable(operation, definition, p.nodeSelectionsVisitor.nodeSuggestions); resolvableReport.HasErrors() {
			p.nodeSelectionsVisitor.hasUnresolvedFields = true

			if i > 100 {
				report.AddInternalError(fmt.Errorf("could not resolve a field: %v", resolvableReport))
				return
			}
		}

		// TODO: what logic should be here?
		if i > 100 {
			report.AddInternalError(fmt.Errorf("something went wrong"))
			return
		}
	}

	if i == 1 {
		// if we have not revisited the operation, we need to check if it is resolvable
		if resolvableReport := p.isResolvable(operation, definition, p.nodeSelectionsVisitor.nodeSuggestions); resolvableReport.HasErrors() {
			p.nodeSelectionsVisitor.hasUnresolvedFields = true
			report.AddInternalError(fmt.Errorf("could not resolve a field: %v", resolvableReport))
		}
	}

	return &NodeSelectionResult{
		dataSources:              p.nodeSelectionsVisitor.dataSources,
		nodeSuggestions:          p.nodeSelectionsVisitor.nodeSuggestions,
		fieldDependsOn:           p.nodeSelectionsVisitor.fieldDependsOn,
		fieldRequirementsConfigs: p.nodeSelectionsVisitor.fieldRequirementsConfigs,
		skipFieldsRefs:           p.nodeSelectionsVisitor.skipFieldsRefs,
	}
}

func (p *NodeSelectionBuilder) isResolvable(operation, definition *ast.Document, nodes *NodeSuggestions) *operationreport.Report {
	p.nodeResolvableVisitor.nodes = nodes
	resolvableReport := &operationreport.Report{}
	p.nodeResolvableWalker.Walk(operation, definition, resolvableReport)

	return resolvableReport
}

func (p *NodeSelectionBuilder) printOperation(operation *ast.Document) {
	var pp string

	if p.config.Debug.PrintOperationEnableASTRefs {
		pp, _ = astprinter.PrintStringIndentDebug(operation, "  ")
	} else {
		pp, _ = astprinter.PrintStringIndent(operation, "  ")
	}

	fmt.Println(pp)
}
