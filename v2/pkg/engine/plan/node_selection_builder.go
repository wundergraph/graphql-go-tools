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

	// fieldMergingAliasRefs maps field refs with a planner generated alias to the
	// original client-visible response name that must be restored in the resolve tree.
	fieldMergingAliasRefs map[int][]byte

	// unresolvableFieldRefs holds field refs whose selection sets were dropped
	// during a rewrite because the abstract type has no possible runtime types
	// able to provide the requested fields. Resolving such fields is always an error.
	unresolvableFieldRefs map[int]struct{}
}

func NewNodeSelectionBuilder(config *Configuration) *NodeSelectionBuilder {
	nodeSelectionsWalker := astvisitor.NewWalkerWithID(48, "NodeSelectionsWalker")
	nodeSelectionVisitor := &nodeSelectionVisitor{
		walker:                        &nodeSelectionsWalker,
		addTypenameInNestedSelections: config.ValidateRequiredExternalFields,
		newFieldRefs:                  make(map[int]struct{}),
		unfetchableFieldRefs:          make(map[int]struct{}),
		unresolvableFieldRefs:         make(map[int]struct{}),
		fieldMergingAliasRefs:         make(map[int][]byte),
	}

	nodeSelectionsWalker.RegisterDocumentVisitor(nodeSelectionVisitor)
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
	p.nodeSelectionsVisitor.unfetchableFieldRefs = make(map[int]struct{})
	p.nodeSelectionsVisitor.unresolvableFieldRefs = make(map[int]struct{})
	p.nodeSelectionsVisitor.fieldMergingAliasRefs = make(map[int][]byte)
}

// SelectNodes implements Steps 1-2 of the planner pipeline.
// It assigns all the fields and their requirements (via @key and @requires) to DataSources.
func (p *NodeSelectionBuilder) SelectNodes(operation, definition *ast.Document, report *operationreport.Report) (out *NodeSelectionResult) {
	dsFilter := NewDataSourceFilter(operation, definition, report, p.config.DataSources, p.nodeSelectionsVisitor.newFieldRefs).
		WithUnfetchableFieldRefs(p.nodeSelectionsVisitor.unfetchableFieldRefs)

	if p.config.Debug.PrintNodeSuggestions {
		dsFilter.EnableSelectionReasons()
	}

	if p.config.Debug.PrintOperationTransformations {
		debugMessage("SelectNodes. Initial operation:\n===========")
		p.printOperation(operation)
	}

	p.nodeSelectionsVisitor.debug = p.config.Debug

	// Step 1. Produce initial suggestions of which datasource owns which fields.
	// We collect info from all subgraphs with the field, plus available keys per path.
	p.nodeSelectionsVisitor.dataSources, p.nodeSelectionsVisitor.nodeSuggestions = dsFilter.FilterDataSources(nil, nil)
	if report.HasErrors() {
		return
	}

	if p.config.Debug.PrintNodeSuggestions {
		p.nodeSelectionsVisitor.nodeSuggestions.printNodesWithFilter("\nInitial node suggestions:\n",
			p.config.Debug.PrintNodeSuggestionsFilterNotSelected)
	}

	// Step 2. For every DataSource-assigned field, check if it has @key or @requires dependencies.
	// Add newly found dependency/required fields into the GraphQL operation.
	p.nodeSelectionsVisitor.secondaryRun = false
	p.nodeSelectionsWalker.Walk(operation, definition, report)
	if report.HasErrors() {
		return
	}

	if p.config.Debug.PrintOperationTransformations {
		debugMessage("SelectNodes. on run #1 operation:")
		p.printOperation(operation)
	}

	i := 1
	hasUnresolvedFields := false
	fallbackKeyJumpsEnabled := false
	refilterWithFallbackKeyJumps := false
	// When the first selection run left unresolved fields (e.g. fields marked with
	// requiresFallbackKey - reachable only via a fallback subset -> compound key jump),
	// enable fallback key jumps and force a full refilter: the datasource selection
	// is redone from scratch with fallback jumps allowed.
	// Fallback jumps are kept behind this failure gate so that plans which
	// succeed with exact key jumps are not affected by the fallback synthesis.
	if !p.nodeSelectionsVisitor.hasNewFields {
		resolvableReport := p.isResolvable(operation, definition, p.nodeSelectionsVisitor.nodeSuggestions)
		if resolvableReport.HasErrors() {
			dsFilter.EnableFallbackKeyJumps()
			fallbackKeyJumpsEnabled = true
			refilterWithFallbackKeyJumps = true
			hasUnresolvedFields = true
		}
	}

	// Additional runs to add paths for the new required fields
	for p.nodeSelectionsVisitor.hasNewFields || hasUnresolvedFields {
		for _, fieldRef := range p.nodeSelectionsVisitor.rewrittenFieldRefs {
			p.nodeSelectionsVisitor.nodeSuggestions.RemoveRewrittenFieldChilds(fieldRef)
		}

		for _, fieldRef := range p.nodeSelectionsVisitor.aliasedFieldRefs {
			p.nodeSelectionsVisitor.nodeSuggestions.AbandonFieldChilds(fieldRef)
		}

		p.nodeSelectionsVisitor.secondaryRun = true

		if p.nodeSelectionsVisitor.hasNewFields || refilterWithFallbackKeyJumps {
			// Repeat Step 1. Update suggestions for the new required fields.
			p.nodeSelectionsVisitor.dataSources, p.nodeSelectionsVisitor.nodeSuggestions = dsFilter.FilterDataSources(p.nodeSelectionsVisitor.fieldLandedTo, p.nodeSelectionsVisitor.fieldRefDependsOn)
			if report.HasErrors() {
				return
			}
			if fallbackKeyJumpsEnabled {
				// the refilter with fallback jumps enabled could land fields on other
				// datasources - drop requirements of the no longer selected (field, ds) pairs
				p.nodeSelectionsVisitor.pruneStaleFieldRequirements()
			}
			refilterWithFallbackKeyJumps = false
		}

		if len(p.nodeSelectionsVisitor.rewrittenFieldRefs) > 0 {
			// The fields unselected after a rewrite could have required fields
			// added to the operation on the parent levels.
			// When such fields were not re-selected on the requiring datasource
			// by the filter run above - their requirements are abandoned,
			// and we have to clean them up.
			p.cleanupAbandonedFieldDependencies(operation)
		}

		if p.config.Debug.PrintOperationTransformations || p.config.Debug.PrintNodeSuggestions {
			debugMessage(fmt.Sprintf("SelectNodes. on run #%d.", i+1))
		}

		if p.config.Debug.PrintNodeSuggestions {
			p.nodeSelectionsVisitor.nodeSuggestions.printNodesWithFilter("\nUpdated node suggestions:\n", p.config.Debug.PrintNodeSuggestionsFilterNotSelected)
		}

		// Repeat Step 2.
		p.nodeSelectionsWalker.Walk(operation, definition, report)
		if report.HasErrors() {
			return
		}

		if p.config.Debug.PrintOperationTransformations {
			debugMessage(fmt.Sprintf("Operation with new required fields (has new fields: %v):", p.nodeSelectionsVisitor.hasNewFields))
			p.printOperation(operation)
		}

		i++

		resolvableReport := p.isResolvable(operation, definition, p.nodeSelectionsVisitor.nodeSuggestions)
		hasUnresolvedFields = resolvableReport.HasErrors()
		if hasUnresolvedFields {
			// same failure gate as before the loop: unresolved fields on a later run
			// (e.g. on the required fields added by the planner) enable fallback key jumps
			if !fallbackKeyJumpsEnabled {
				dsFilter.EnableFallbackKeyJumps()
				fallbackKeyJumpsEnabled = true
				refilterWithFallbackKeyJumps = true
			}

			if i > 100 {
				report.AddInternalError(fmt.Errorf("could not resolve a field: %v", resolvableReport))
				return
			}
			continue
		}

		// if we have revisited operation more than 100 times, we have a bug
		if i > 100 {
			report.AddInternalError(fmt.Errorf("something went wrong"))
			return
		}
	}

	p.nodeSelectionsVisitor.nodeSuggestions.ProcessDefer(p.nodeSelectionsVisitor.fieldRequirementsConfigs)

	return &NodeSelectionResult{
		dataSources:              p.nodeSelectionsVisitor.dataSources,
		nodeSuggestions:          p.nodeSelectionsVisitor.nodeSuggestions,
		fieldDependsOn:           p.nodeSelectionsVisitor.fieldDependsOn,
		fieldRequirementsConfigs: p.nodeSelectionsVisitor.fieldRequirementsConfigs,
		skipFieldsRefs:           p.nodeSelectionsVisitor.skipFieldsRefs,
		fieldRefDependsOn:        p.nodeSelectionsVisitor.fieldRefDependsOn,
		fieldDependencyKind:      p.nodeSelectionsVisitor.fieldDependencyKind,
		fieldMergingAliasRefs:    p.nodeSelectionsVisitor.fieldMergingAliasRefs,
		unresolvableFieldRefs:    p.nodeSelectionsVisitor.unresolvableFieldRefs,
	}
}

// cleanupAbandonedFieldDependencies is a mirror of the field requirements registration.
// When a field is no longer selected on the datasource which required the fields
// added to the operation by the planner, its requirements are abandoned:
// we remove the dependency mappings, and when a required field is not needed
// by any other field anymore - we remove it from the operation
// and orphan its suggestions.
func (p *NodeSelectionBuilder) cleanupAbandonedFieldDependencies(operation *ast.Document) {
	v := p.nodeSelectionsVisitor

	// requirements of the nested key jumps depend on the key fields of the previous jump,
	// so removing a required field could abandon other dependency entries -
	// repeat until there is nothing to remove
	for {
		abandonedRequiredRefs := make(map[int]struct{})

		for key, requiredRefs := range v.fieldDependsOn {
			if v.nodeSuggestions.IsSelectedOnDataSource(key.fieldRef, key.dsHash) {
				continue
			}

			delete(v.fieldDependsOn, key)
			delete(v.fieldRequirementsConfigs, key)

			for _, requiredRef := range requiredRefs {
				abandonedRequiredRefs[requiredRef] = struct{}{}
			}
		}

		if len(abandonedRequiredRefs) == 0 {
			return
		}

		// rebuild the plain field refs dependency index from the remaining entries
		v.fieldRefDependsOn = make(map[int][]int, len(v.fieldRefDependsOn))
		stillRequiredRefs := make(map[int]struct{})
		for key, requiredRefs := range v.fieldDependsOn {
			v.fieldRefDependsOn[key.fieldRef] = append(v.fieldRefDependsOn[key.fieldRef], requiredRefs...)
			for _, requiredRef := range requiredRefs {
				stillRequiredRefs[requiredRef] = struct{}{}
			}
		}

		for kindKey := range v.fieldDependencyKind {
			if !slices.Contains(v.fieldRefDependsOn[kindKey.field], kindKey.dependsOn) {
				delete(v.fieldDependencyKind, kindKey)
			}
		}

		touchedSelectionSets := make(map[int]struct{})
		for requiredRef := range abandonedRequiredRefs {
			if _, stillRequired := stillRequiredRefs[requiredRef]; stillRequired {
				continue
			}

			delete(v.fieldLandedTo, requiredRef)
			v.skipFieldsRefs = slices.DeleteFunc(v.skipFieldsRefs, func(ref int) bool { return ref == requiredRef })
			v.nodeSuggestions.OrphanSuggestionsForFieldRef(requiredRef)
			if setRef := removeFieldFromOperationSelectionSets(operation, requiredRef); setRef != ast.InvalidRef {
				touchedSelectionSets[setRef] = struct{}{}
			}
		}

		// The key fields are added to the operation along with an accompanying __typename selection,
		// which is intentionally not tracked as a required field.
		// When a selection set has no required fields anymore,
		// the planner added __typename is abandoned as well - remove it too.
		for setRef := range touchedSelectionSets {
			p.removeAbandonedTypenameFromSelectionSet(operation, setRef, stillRequiredRefs)
		}
	}
}

func (p *NodeSelectionBuilder) removeAbandonedTypenameFromSelectionSet(operation *ast.Document, setRef int, stillRequiredRefs map[int]struct{}) {
	v := p.nodeSelectionsVisitor

	typenameRefs := make([]int, 0, 1)
	for _, selectionRef := range operation.SelectionSets[setRef].SelectionRefs {
		selection := operation.Selections[selectionRef]
		if selection.Kind != ast.SelectionKindField {
			continue
		}

		if _, stillRequired := stillRequiredRefs[selection.Ref]; stillRequired {
			// the selection set still has required fields, __typename is still needed
			return
		}

		if operation.FieldNameUnsafeString(selection.Ref) == typeNameField && slices.Contains(v.skipFieldsRefs, selection.Ref) {
			typenameRefs = append(typenameRefs, selection.Ref)
		}
	}

	for _, typenameRef := range typenameRefs {
		v.skipFieldsRefs = slices.DeleteFunc(v.skipFieldsRefs, func(ref int) bool { return ref == typenameRef })
		v.nodeSuggestions.OrphanSuggestionsForFieldRef(typenameRef)
		removeFieldFromOperationSelectionSets(operation, typenameRef)
	}
}

// removeFieldFromOperationSelectionSets removes the field from the selection set containing it
// and returns the ref of that selection set, or ast.InvalidRef when the field was not found.
// We have to iterate over the selection sets, because the field could have been added
// not only to a field selection set but also to a planner created inline fragment.
func removeFieldFromOperationSelectionSets(operation *ast.Document, fieldRef int) int {
	fieldNode := ast.Node{Kind: ast.NodeKindField, Ref: fieldRef}

	for setRef := range operation.SelectionSets {
		if operation.RemoveNodeFromSelectionSet(setRef, fieldNode) {
			return setRef
		}
	}

	return ast.InvalidRef
}

func (p *NodeSelectionBuilder) isResolvable(operation, definition *ast.Document, nodes *NodeSuggestions) *operationreport.Report {
	p.nodeResolvableVisitor.nodes = nodes
	p.nodeResolvableVisitor.unfetchableFieldRefs = p.nodeSelectionsVisitor.unfetchableFieldRefs
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
