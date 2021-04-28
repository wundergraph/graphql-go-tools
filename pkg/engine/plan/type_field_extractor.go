package plan

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
)

const (
	federationKeyDirectiveName      = "key"
	federationRequireDirectiveName  = "requires"
	federationExternalDirectiveName = "external"
)

// TypeFieldExtractor takes an ast.Document as input
// and generates the TypeField configuration for both root fields & child fields
// If a type is a federation entity (annotated with @key directive)
// and a field is is extended, this field will be skipped
// so that only "local" fields will be generated
type TypeFieldExtractor struct {
	document *ast.Document
}

func NewNodeExtractor(document *ast.Document) *TypeFieldExtractor {
	return &TypeFieldExtractor{document: document}
}

// GetAllNodes returns all Root- & ChildNodes
func (r *TypeFieldExtractor) GetAllNodes() (rootNodes, childNodes []TypeField) {
	rootNodes = r.getAllRootNodes()
	childNodes = r.getAllChildNodes(rootNodes)
	return
}

func (r *TypeFieldExtractor) getAllRootNodes() []TypeField {
	var rootNodes []TypeField

	for _, astNode := range r.document.RootNodes {
		switch astNode.Kind {
		case ast.NodeKindObjectTypeExtension, ast.NodeKindObjectTypeDefinition:
			r.addRootNodes(astNode, &rootNodes)
		}
	}

	return rootNodes
}

func (r *TypeFieldExtractor) getAllChildNodes(rootNodes []TypeField) []TypeField {
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

			fieldTypeName := r.document.NodeNameString(r.document.FieldDefinitionTypeNode(fieldRef))
			r.findChildNodesForType(fieldTypeName, &childNodes)
		}
	}

	return childNodes
}

func (r *TypeFieldExtractor) findChildNodesForType(typeName string, childNodes *[]TypeField) {
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

		fieldTypeName := r.document.NodeNameString(r.document.FieldDefinitionTypeNode(fieldRef))
		r.findChildNodesForType(fieldTypeName, childNodes)
	}
}

func (r *TypeFieldExtractor) addChildTypeFieldName(typeName, fieldName string, childNodes *[]TypeField) bool {
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

func (r *TypeFieldExtractor) addRootNodes(astNode ast.Node, rootNodes *[]TypeField) {
	typeName := r.document.NodeNameString(astNode)
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

func (r *TypeFieldExtractor) isRootOperationTypeName(typeName string) bool {
	rootOperationNames := map[string]struct{}{
		"Query":        {},
		"Mutation":     {},
		"Subscription": {},
	}

	_, exists := rootOperationNames[typeName]

	return exists
}

func (r *TypeFieldExtractor) isEntity(astNode ast.Node) bool {
	directiveRefs := r.document.NodeDirectives(astNode)

	for _, directiveRef := range directiveRefs {
		if directiveName := r.document.DirectiveNameString(directiveRef); directiveName == federationKeyDirectiveName {
			return true
		}
	}

	return false
}
