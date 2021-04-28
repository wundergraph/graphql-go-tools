package plan

import (
	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafebytes"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
)

const (
	federationKeyDirectiveName      = "key"
	federationRequireDirectiveName  = "requires"
	federationExternalDirectiveName = "external"
)

// Extract all fields from Entities.
// If the Entity is extended - only local fields (fields without external directive).

type nodeExtractor struct {
	document *ast.Document
}

func NewNodeExtractor(document *ast.Document) *nodeExtractor {
	return &nodeExtractor{document: document}
}

func (r *nodeExtractor) GetAllNodes() (rootNodes, childNodes []TypeField) {
	rootNodes = r.getAllRootNodes()
	childNodes = r.getAllChildNodes(rootNodes)

	return
}

func (r *nodeExtractor) getAllRootNodes() []TypeField {
	var rootNodes []TypeField

	for _, astNode := range r.document.RootNodes {
		switch astNode.Kind {
		case ast.NodeKindObjectTypeExtension, ast.NodeKindObjectTypeDefinition:
			r.addRootNodes(astNode, &rootNodes)
		}
	}

	return rootNodes
}

func (r *nodeExtractor) getAllChildNodes(rootNodes []TypeField) []TypeField {
	var childNodes []TypeField

	for i := range rootNodes {
		fieldNameToRef := make(map[string]int, len(rootNodes[i].FieldNames))

		rootNodeASTNode, exists := r.document.Index.FirstNodeByNameStr(rootNodes[i].TypeName)
		if !exists {
			continue
		}

		fieldRefs := r.document.NodeFieldDefinitions(rootNodeASTNode)
		for _, fieldRef := range fieldRefs {
			fieldName := r.document.FieldDefinitionNameString(fieldRef)
			fieldNameToRef[fieldName] = fieldRef
		}

		for _, fieldName := range rootNodes[i].FieldNames {
			fieldRef := fieldNameToRef[fieldName]

			fieldTypeName := r.getNodeName(r.document.FieldDefinitionTypeNode(fieldRef))
			r.findChildNodesForType(fieldTypeName, &childNodes)
		}
	}

	return childNodes
}

func (r *nodeExtractor) findChildNodesForType(typeName string, childNodes *[]TypeField) {
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

		fieldTypeName := r.getNodeName(r.document.FieldDefinitionTypeNode(fieldRef))
		r.findChildNodesForType(fieldTypeName, childNodes)
	}
}

func (r *nodeExtractor) addChildTypeFieldName(typeName, fieldName string, childNodes *[]TypeField) bool {
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

	*childNodes = append(*childNodes, TypeField{
		TypeName:   typeName,
		FieldNames: []string{fieldName},
	})

	return true
}

func (r *nodeExtractor) addRootNodes(astNode ast.Node, rootNodes *[]TypeField) {
	typeName := r.getNodeName(astNode)
	if !r.isEntity(astNode) && !r.isRootOperationTypeName(typeName) {
		return
	}

	var fieldNames []string

	fieldRefs := r.document.NodeFieldDefinitions(astNode)
	for _, fieldRef := range fieldRefs {
		// check if field definition is external (has external directive)
		if r.document.FieldDefinitionHasNamedDirective(fieldRef,federationExternalDirectiveName) {
			continue
		}

		fieldName := r.document.FieldDefinitionNameString(fieldRef)
		fieldNames = append(fieldNames, fieldName)
	}

	if len(fieldNames) == 0 {
		return
	}

	*rootNodes = append(*rootNodes, TypeField{
		TypeName:   typeName,
		FieldNames: fieldNames,
	})
}

// document.NodeNameBytes method doesnt support NodeKindObjectTypeExtension and NodeKindInterfaceTypeExtension
func (r *nodeExtractor) getNodeName(astNode ast.Node) string {
	var ref ast.ByteSliceReference

	switch astNode.Kind {
	case ast.NodeKindObjectTypeDefinition:
		ref = r.document.ObjectTypeDefinitions[astNode.Ref].Name
	case ast.NodeKindObjectTypeExtension:
		ref = r.document.ObjectTypeExtensions[astNode.Ref].Name
	case ast.NodeKindInterfaceTypeExtension:
		ref = r.document.InterfaceTypeExtensions[astNode.Ref].Name
	case ast.NodeKindInterfaceTypeDefinition:
		ref = r.document.InterfaceTypeDefinitions[astNode.Ref].Name
	case ast.NodeKindInputObjectTypeDefinition:
		ref = r.document.InputObjectTypeDefinitions[astNode.Ref].Name
	case ast.NodeKindUnionTypeDefinition:
		ref = r.document.UnionTypeDefinitions[astNode.Ref].Name
	case ast.NodeKindScalarTypeDefinition:
		ref = r.document.ScalarTypeDefinitions[astNode.Ref].Name
	case ast.NodeKindDirectiveDefinition:
		ref = r.document.DirectiveDefinitions[astNode.Ref].Name
	case ast.NodeKindField:
		ref = r.document.Fields[astNode.Ref].Name
	case ast.NodeKindDirective:
		ref = r.document.Directives[astNode.Ref].Name
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
		if directiveName := r.document.DirectiveNameString(directiveRef); directiveName == federationKeyDirectiveName {
			return true
		}
	}

	return false
}
