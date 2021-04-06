package federation

import (
	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafebytes"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
)

// Extract all fields from Entities.
// If the Entity is extended - only local fields (fields without external directive).

type nodeExtractor struct {
	document *ast.Document
}

func newNodeExtractor(document *ast.Document) *nodeExtractor {
	return &nodeExtractor{document: document}
}

func (r *nodeExtractor) getAllNodes() (rootNodes, childNodes []plan.TypeField) {
	rootNodes = r.getAllRootNodes()
	childNodes = r.getAllChildNodes(rootNodes)

	return
}

func (r *nodeExtractor) getAllRootNodes() []plan.TypeField {
	var rootNodes []plan.TypeField

	for _, astNode := range r.document.RootNodes {
		switch astNode.Kind {
		case ast.NodeKindObjectTypeExtension, ast.NodeKindObjectTypeDefinition:
			r.addRootNodes(astNode, &rootNodes)
		}
	}

	return rootNodes
}

func (r *nodeExtractor) getAllChildNodes(rootNodes []plan.TypeField) []plan.TypeField {
	var childNodes []plan.TypeField

	for i := range rootNodes {
		for _, fieldName := range rootNodes[i].FieldNames {
			fieldNode, exists := r.document.Index.FirstNodeByNameStr(fieldName)
			if !exists {
				continue
			}

			fieldTypeName := r.document.FieldDefinitionTypeNode(fieldNode.Ref).NameString(r.document)
			r.findChildNodesForType(fieldTypeName, &childNodes)
		}
	}

	return childNodes
}

func (r *nodeExtractor) findChildNodesForType(typeName string, childNodes *[]plan.TypeField) {
	node, exists := r.document.Index.FirstNodeByNameStr(typeName)
	if !exists {
		return
	}

	fieldsRefs := r.document.NodeFieldDefinitions(node)

	for _, fieldRef := range fieldsRefs {
		fieldName := r.document.FieldDefinitionNameString(fieldRef)

		if added := r.addChildTypeFieldName(typeName, fieldName, childNodes); !added {
			continue
		}

		fieldTypeName := r.document.FieldDefinitionTypeNode(fieldRef).NameString(r.document)
		r.findChildNodesForType(fieldTypeName, childNodes)
	}
}

func (r *nodeExtractor) addChildTypeFieldName(typeName, fieldName string, childNodes *[]plan.TypeField) bool {
	for i := range *childNodes {
		if (*childNodes)[i].TypeName != typeName {
			continue
		}

		for _, field := range (*childNodes)[i].FieldNames {
			if field == fieldName {
				return false
			}
		}

		(*childNodes)[i].FieldNames = append((*childNodes)[i].FieldNames, fieldName)
		return true
	}

	*childNodes = append(*childNodes, plan.TypeField{
		TypeName:   typeName,
		FieldNames: []string{fieldName},
	})

	return true
}

func (r *nodeExtractor) addRootNodes(astNode ast.Node, rootNodes *[]plan.TypeField) {
	typeName := r.getNodeName(astNode)
	if !r.isEntity(astNode) && !r.isRootOperationTypeName(typeName) {
		return
	}

	var fieldNames []string

	fieldRefs := r.document.NodeFieldDefinitions(astNode)
	for _, fieldRef := range fieldRefs {
		if isExternalField(r.document, fieldRef) {
			continue
		}

		fieldName := r.document.FieldDefinitionNameString(fieldRef)
		fieldNames = append(fieldNames, fieldName)
	}

	if len(fieldNames) == 0 {
		return
	}

	*rootNodes = append(*rootNodes, plan.TypeField{
		TypeName:   typeName,
		FieldNames: fieldNames,
	})
}

func (r *nodeExtractor) getNodeName(astNode ast.Node) string {
	var ref ast.ByteSliceReference

	switch astNode.Kind {
	case ast.NodeKindObjectTypeDefinition:
		ref = r.document.ObjectTypeDefinitions[astNode.Ref].Name
	case ast.NodeKindObjectTypeExtension:
		ref = r.document.ObjectTypeExtensions[astNode.Ref].Name
	}

	bytesName := r.document.Input.ByteSlice(ref)

	return unsafebytes.BytesToString(bytesName)
}

func (r *nodeExtractor) isRootOperationTypeName(typeName string) bool {
	rootOperationNames := map[string]struct{}{
		"Query":        {},
		"Mutation":     {},
		"Subscription": {},
	}

	_, exists := rootOperationNames[typeName]

	return exists
}

func (r *nodeExtractor) isEntity(astNode ast.Node) bool {
	directiveRefs := r.document.NodeDirectives(astNode)

	for _, directiveRef := range directiveRefs {
		if directiveName := r.document.DirectiveNameString(directiveRef); directiveName == keyDirectiveName {
			return true
		}
	}

	return false
}
