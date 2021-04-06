package planconfiguration

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
)

type TypeField struct {
	TypeName   string
	FieldNames []string
}

func ExtractRootNodes(document *ast.Document) []TypeField {
	extractor := NewRootNodeExtractor(document)
	return extractor.GetAllRootNodes()
}

// Extract all fields from Entities.
// If the Entity is extended - only local fields (fields without external directive).

type RootNodeExtractor struct {
	document *ast.Document
}

func NewRootNodeExtractor(document *ast.Document) *RootNodeExtractor {
	return &RootNodeExtractor{document: document}
}

func (r *RootNodeExtractor) GetAllRootNodes() []TypeField {
	var rootNodes []TypeField

	for _, objectTypeExt := range r.document.ObjectTypeExtensions {
		r.addRootNodesForObjectDefinition(objectTypeExt.ObjectTypeDefinition, &rootNodes)
	}

	for _, objectType := range r.document.ObjectTypeDefinitions {
		r.addRootNodesForObjectDefinition(objectType, &rootNodes)
	}

	return rootNodes
}

func (r *RootNodeExtractor) addRootNodesForObjectDefinition(objectType ast.ObjectTypeDefinition, rootNodes *[]TypeField) {
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

	*rootNodes = append(*rootNodes, TypeField{
		TypeName:   typeName,
		FieldNames: fieldNames,
	})
}

func (r *RootNodeExtractor) isRootOperationTypeName(typeName string) bool {
	rootOperationNames := map[string]struct{}{
		"Query":        {},
		"Mutation":     {},
		"Subscription": {},
	}

	_, ok := rootOperationNames[typeName]

	return ok
}
