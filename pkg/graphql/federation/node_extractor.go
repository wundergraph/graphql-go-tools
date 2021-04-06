package federation

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
)

// Extract all fields from Entities.
// If the Entity is extended - only local fields (fields without external directive).

type rootNodeExtractor struct {
	document *ast.Document
}

func NewRootNodeExtractor(document *ast.Document) *rootNodeExtractor {
	return &rootNodeExtractor{document: document}
}

func (r *rootNodeExtractor) getAllRootNodes() []plan.TypeField {
	var rootNodes []plan.TypeField

	for _, objectTypeExt := range r.document.ObjectTypeExtensions {
		r.addRootNodesForObjectDefinition(objectTypeExt.ObjectTypeDefinition, &rootNodes)
	}

	for _, objectType := range r.document.ObjectTypeDefinitions {
		r.addRootNodesForObjectDefinition(objectType, &rootNodes)
	}

	return rootNodes
}

func (r *rootNodeExtractor) addRootNodesForObjectDefinition(objectType ast.ObjectTypeDefinition, rootNodes *[]plan.TypeField) {
	typeName := r.document.Input.ByteSliceString(objectType.Name)

	if !isEntity(r.document, objectType) && !r.isRootOperationTypeName(typeName) {
		return
	}

	var fieldNames []string

	for _, fieldRef := range objectType.FieldsDefinition.Refs {
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

func (r *rootNodeExtractor) isRootOperationTypeName(typeName string) bool {
	rootOperationNames := map[string]struct{}{
		"Query":        {},
		"Mutation":     {},
		"Subscription": {},
	}

	_, ok := rootOperationNames[typeName]

	return ok
}
