package plan

import (
	"fmt"
	"io"
	"slices"

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

type fieldDependencyKind int

const (
	fieldDependencyKindKey fieldDependencyKind = iota
	fieldDependencyKindRequires
)

type NodeSelectionResult struct {
	// data sources configurations, used by the current operation
	dataSources []DataSource

	// nodeSuggestions holds information about suggested data sources for each field
	nodeSuggestions *NodeSuggestions

	// fieldDependsOn maps fieldIndexKey to a list of fields refs. Those fields should be planned
	// before the fieldIndexKey.fieldRef.
	fieldDependsOn map[fieldIndexKey][]int

	// fieldRequirementsConfigs maps fieldIndexKey to a list of required configurations that are
	// used later to build representation variables.
	fieldRequirementsConfigs map[fieldIndexKey][]FederationFieldConfiguration

	// skipFieldsRefs holds required field refs added by the planner.
	// These fields should not be added to user response.
	skipFieldsRefs []int

	fieldRefDependsOn   map[int][]int
	fieldDependencyKind map[fieldDependencyKey]fieldDependencyKind
}

func NewNodeSelectionBuilder(config *Configuration) *NodeSelectionBuilder {
	nodeSelectionsWalker := astvisitor.NewWalkerWithID(48, "NodeSelectionsWalker")
	nodeSelectionVisitor := &nodeSelectionVisitor{
		walker:                        &nodeSelectionsWalker,
		addTypenameInNestedSelections: config.ValidateRequiredExternalFields,
		newFieldRefs:                  make(map[int]struct{}),
	}

	nodeSelectionsWalker.RegisterEnterDocumentVisitor(nodeSelectionVisitor)
	nodeSelectionsWalker.RegisterFieldVisitor(nodeSelectionVisitor)
	nodeSelectionsWalker.RegisterEnterOperationVisitor(nodeSelectionVisitor)
	nodeSelectionsWalker.RegisterSelectionSetVisitor(nodeSelectionVisitor)

	nodeResolvableWalker := astvisitor.NewWalkerWithID(32, "NodeResolvableWalker")
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

func (p *NodeSelectionBuilder) SetOperationName(name string) {
	p.nodeSelectionsVisitor.operationName = name
}

func (p *NodeSelectionBuilder) ResetSkipFieldRefs() {
	p.nodeSelectionsVisitor.skipFieldsRefs = nil
	p.nodeSelectionsVisitor.newFieldRefs = make(map[int]struct{})
}

func (p *NodeSelectionBuilder) SelectNodes(operation, definition *ast.Document, report *operationreport.Report) (out *NodeSelectionResult) {
	dsFilter := NewDataSourceFilter(operation, definition, report, p.config.DataSources, p.nodeSelectionsVisitor.newFieldRefs)

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
		dsFilter.FilterDataSources(nil, nil)
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
				dsFilter.FilterDataSources(p.nodeSelectionsVisitor.fieldLandedTo, p.nodeSelectionsVisitor.fieldRefDependsOn)
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
		fieldRefDependsOn:        p.nodeSelectionsVisitor.fieldRefDependsOn,
		fieldDependencyKind:      p.nodeSelectionsVisitor.fieldDependencyKind,
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
		pp, _ = astprinter.PrintStringIndentDebug(operation, "  ", func(fieldRef int, out io.Writer) {
			if p.config.Debug.PrintNodeSuggestions {
				if p.nodeSelectionsVisitor.nodeSuggestions == nil {
					return
				}

				treeNodeId := TreeNodeID(fieldRef)
				node, ok := p.nodeSelectionsVisitor.nodeSuggestions.responseTree.Find(treeNodeId)
				if !ok {
					return
				}

				items := node.GetData()
				for _, id := range items {
					if p.nodeSelectionsVisitor.nodeSuggestions.items[id].Selected {
						_, _ = fmt.Fprintf(out, "  %s", p.nodeSelectionsVisitor.nodeSuggestions.items[id].StringShort())
					}
				}
			}

			if slices.Contains(p.nodeSelectionsVisitor.skipFieldsRefs, fieldRef) {
				_, _ = fmt.Fprintf(out, "  (skip)")
			}
		})
	} else {
		pp, _ = astprinter.PrintStringIndent(operation, "    ")
	}

	fmt.Println(pp)
}
