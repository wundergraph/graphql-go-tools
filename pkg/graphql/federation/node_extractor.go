package federation

import (
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
		typeName := r.document.NodeNameString(astNode)
		r.addRootNodesForObjectDefinition(typeName, &rootNodes)
	}

	return rootNodes
}

func (r *nodeExtractor) getAllChildNodes(rootNodes []plan.TypeField) []plan.TypeField {
	var childNodes []plan.TypeField

	for i := range rootNodes {
		for _, fieldName := range rootNodes[i].FieldNames {
			fieldNode, ok := r.document.Index.FirstNodeByNameStr(fieldName)
			if !ok {
				continue
			}

			fieldTypeName := r.document.FieldDefinitionTypeNode(fieldNode.Ref).NameString(r.document)
			r.findChildNodesForType(fieldTypeName, &childNodes)
		}
	}

	return childNodes
}

func (r *nodeExtractor) findChildNodesForType(typeName string, childNodes *[]plan.TypeField) {
	fieldsRefs := r.getTypeFieldRefs(typeName)

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

func (r *nodeExtractor) addRootNodesForObjectDefinition(typeName string, rootNodes *[]plan.TypeField) {
	if !r.isEntity(typeName) && !r.isRootOperationTypeName(typeName) {
		return
	}

	var fieldNames []string

	fieldRefs := r.getTypeFieldRefs(typeName)
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

func (r *nodeExtractor) getTypeFieldRefs(typeName string) []int {
	node, ok := r.document.Index.FirstNodeByNameStr(typeName)
	if !ok {
		return nil
	}

	var fields []int
	switch node.Kind {
	case ast.NodeKindObjectTypeDefinition:
		fields = r.document.ObjectTypeDefinitions[node.Ref].FieldsDefinition.Refs
	case ast.NodeKindInterfaceTypeDefinition:
		fields = r.document.InterfaceTypeDefinitions[node.Ref].FieldsDefinition.Refs
	case ast.NodeKindObjectTypeExtension:
		fields = r.document.ObjectTypeExtensions[node.Ref].FieldsDefinition.Refs
	case ast.NodeKindInterfaceTypeExtension:
		fields = r.document.InterfaceTypeExtensions[node.Ref].FieldsDefinition.Refs
	default:
		return nil
	}

	return fields
}

func (r *nodeExtractor) getTypeDirectiveRefs(typeName string) []int {
	node, ok := r.document.Index.FirstNodeByNameStr(typeName)
	if !ok {
		return nil
	}

	var directives []int

	switch node.Kind {
	case ast.NodeKindObjectTypeDefinition:
		directives = r.document.ObjectTypeDefinitions[node.Ref].Directives.Refs
	case ast.NodeKindInterfaceTypeDefinition:
		directives = r.document.InterfaceTypeDefinitions[node.Ref].Directives.Refs
	case ast.NodeKindObjectTypeExtension:
		directives = r.document.ObjectTypeExtensions[node.Ref].Directives.Refs
	case ast.NodeKindInterfaceTypeExtension:
		directives = r.document.InterfaceTypeExtensions[node.Ref].Directives.Refs
	default:
		return nil
	}

	return directives
}

func (r *nodeExtractor) isRootOperationTypeName(typeName string) bool {
	rootOperationNames := map[string]struct{}{
		"Query":        {},
		"Mutation":     {},
		"Subscription": {},
	}

	_, ok := rootOperationNames[typeName]

	return ok
}

func (r *nodeExtractor) isEntity(typeName string) bool {
	directiveRefs := r.getTypeDirectiveRefs(typeName)

	for _, directiveRef := range directiveRefs {
		if directiveName := r.document.DirectiveNameString(directiveRef); directiveName == keyDirectiveName {
			return true
		}
	}

	return false
}
