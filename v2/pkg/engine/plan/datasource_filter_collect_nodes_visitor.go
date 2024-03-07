package plan

import (
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

type collectNodesVisitor struct {
	operation    *ast.Document
	definition   *ast.Document
	walker       *astvisitor.Walker
	secondaryRun bool

	dataSources   []DataSource
	nodes         *NodeSuggestions
	hints         []NodeSuggestionHint
	parentNodeIds []uint

	saveSelectionReason bool
}

func (f *collectNodesVisitor) EnterDocument(_, _ *ast.Document) {
	f.parentNodeIds = []uint{treeRootID}

	if !f.secondaryRun {
		f.nodes = NewNodeSuggestions()
		return
	}

	if f.nodes == nil {
		panic("nodes should not be nil")
	}
}

func (f *collectNodesVisitor) EnterField(ref int) {
	if f.nodes.IsFieldSeen(ref) {
		currentNodeId := TreeNodeID(ref)
		f.parentNodeIds = append(f.parentNodeIds, currentNodeId)
		return
	}
	f.nodes.AddSeenField(ref)

	typeName := f.walker.EnclosingTypeDefinition.NameString(f.definition)
	fieldName := f.operation.FieldNameUnsafeString(ref)
	fieldAliasOrName := f.operation.FieldAliasOrNameString(ref)

	isTypeName := fieldName == typeNameField
	parentPath := f.walker.Path.DotDelimitedString()
	onFragment := f.walker.Path.EndsWithFragment()
	var parentPathWithoutFragment *string
	if onFragment {
		p := f.walker.Path[:len(f.walker.Path)-1].DotDelimitedString()
		parentPathWithoutFragment = &p
	}

	currentPath := parentPath + "." + fieldAliasOrName

	itemIds := make([]int, 0, 1)

	for _, v := range f.dataSources {
		hasRootNode := v.HasRootNode(typeName, fieldName) || (isTypeName && v.HasRootNodeWithTypename(typeName))
		hasChildNode := v.HasChildNode(typeName, fieldName) || (isTypeName && v.HasChildNodeWithTypename(typeName))

		allowTypeName := true
		// we should not select a typename on the interface object
		for _, k := range v.FederationConfiguration().InterfaceObjects {
			if k.InterfaceTypeName == typeName || slices.Contains(k.ConcreteTypeNames, typeName) {
				allowTypeName = false
				break
			}
		}

		lessPreferable := false
		if hasRootNode {
			for _, k := range v.FederationConfiguration().Keys {
				if k.TypeName == typeName && k.DisableEntityResolver {
					lessPreferable = true
					break
				}
			}
		}

		if !allowTypeName && isTypeName {
			continue
		}

		if hasRootNode || hasChildNode {
			node := NodeSuggestion{
				TypeName:                  typeName,
				FieldName:                 fieldName,
				DataSourceHash:            v.Hash(),
				Path:                      currentPath,
				ParentPath:                parentPath,
				IsRootNode:                hasRootNode,
				onFragment:                onFragment,
				parentPathWithoutFragment: parentPathWithoutFragment,
				fieldRef:                  ref,
				LessPreferable:            lessPreferable,
			}

			f.nodes.addSuggestion(&node)

			itemIds = append(itemIds, len(f.nodes.items)-1)
		}
	}

	parentNodeId := f.currentParentID()
	currentNodeId := TreeNodeID(ref)

	// we intentionally ignore the return values added, exists
	// because we do not revisit the same field refs, so all added nodes should be new and unique
	_, _ = f.nodes.responseTree.Add(currentNodeId, parentNodeId, itemIds)
	f.parentNodeIds = append(f.parentNodeIds, currentNodeId)
}

func (f *collectNodesVisitor) currentParentID() uint {
	return f.parentNodeIds[len(f.parentNodeIds)-1]
}

func TreeNodeID(fieldRef int) uint {
	// we add 100 to the fieldRef to make sure that the tree node id is never 0
	// cause 0 is a valid field ref
	// but for tree 0 is reserved for the root node
	return uint(100 + fieldRef)
}

func (f *collectNodesVisitor) LeaveField(ref int) {
	parentNodeId := f.currentParentID()
	currentNodeId := TreeNodeID(ref)

	if parentNodeId == currentNodeId {
		f.parentNodeIds = f.parentNodeIds[:len(f.parentNodeIds)-1]
	}
}
