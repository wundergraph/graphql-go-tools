package federationdata

import (
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/federation/sdlmerge"
)

var fieldsArgumentNameBytes = []byte("fields")

// RequiredFieldExtractor extracts all required fields from an ast.Document
// containing a parsed federation subgraph SDL
// by visiting the directives specified in the federation specification
// and extracting the required metadata
type RequiredFieldExtractor struct {
	document *ast.Document
}

func NewRequiredFieldExtractor(document *ast.Document) *RequiredFieldExtractor {
	return &RequiredFieldExtractor{
		document: document,
	}
}

func (f *RequiredFieldExtractor) GetAllRequiredFields() plan.FieldConfigurations {
	var fieldRequires plan.FieldConfigurations

	f.addFieldsForObjectExtensionDefinitions(&fieldRequires)
	f.addFieldsForObjectDefinitions(&fieldRequires)

	return fieldRequires
}

func (f *RequiredFieldExtractor) addFieldsForObjectExtensionDefinitions(fieldRequires *plan.FieldConfigurations) {
	for _, objectTypeExt := range f.document.ObjectTypeExtensions {
		objectType := objectTypeExt.ObjectTypeDefinition
		typeName := f.document.Input.ByteSliceString(objectType.Name)

		primaryKeys, exists := f.primaryKeyFieldsIfObjectTypeIsEntity(objectType)
		if !exists {
			continue
		}

		for _, fieldDefinitionRef := range objectType.FieldsDefinition.Refs {
			if f.document.FieldDefinitionHasNamedDirective(fieldDefinitionRef, sdlmerge.ExternalDirectiveName) {
				continue
			}

			fieldName := f.document.FieldDefinitionNameString(fieldDefinitionRef)

			requiredFields := make([]string, len(primaryKeys))
			copy(requiredFields, primaryKeys)

			requiredFieldsByRequiresDirective := requiredFieldsByRequiresDirective(f.document, fieldDefinitionRef)
			requiredFields = append(requiredFields, requiredFieldsByRequiresDirective...)

			*fieldRequires = append(*fieldRequires, plan.FieldConfiguration{
				TypeName:       typeName,
				FieldName:      fieldName,
				RequiresFields: requiredFields,
			})
		}
	}
}

func (f *RequiredFieldExtractor) addFieldsForObjectDefinitions(fieldRequires *plan.FieldConfigurations) {
	for _, objectType := range f.document.ObjectTypeDefinitions {
		typeName := f.document.Input.ByteSliceString(objectType.Name)

		primaryKeys, exists := f.primaryKeyFieldsIfObjectTypeIsEntity(objectType)
		if !exists {
			continue
		}

		primaryKeysSet := make(map[string]struct{}, len(primaryKeys))
		for _, val := range primaryKeys {
			primaryKeysSet[val] = struct{}{}
		}

		for _, fieldRef := range objectType.FieldsDefinition.Refs {
			fieldName := f.document.FieldDefinitionNameString(fieldRef)
			if _, exists := primaryKeysSet[fieldName]; exists { // Field is part of primary key, it couldn't have any required fields
				continue
			}

			requiredFields := make([]string, len(primaryKeys))
			copy(requiredFields, primaryKeys)

			*fieldRequires = append(*fieldRequires, plan.FieldConfiguration{
				TypeName:       typeName,
				FieldName:      fieldName,
				RequiresFields: requiredFields,
			})
		}
	}
}

func requiredFieldsByRequiresDirective(document *ast.Document, fieldDefinitionRef int) []string {
	for _, directiveRef := range document.FieldDefinitions[fieldDefinitionRef].Directives.Refs {
		if directiveName := document.DirectiveNameString(directiveRef); directiveName != sdlmerge.RequireDirectiveName {
			continue
		}

		value, exists := document.DirectiveArgumentValueByName(directiveRef, fieldsArgumentNameBytes)
		if !exists {
			continue
		}
		if value.Kind != ast.ValueKindString {
			continue
		}

		fieldsStr := document.StringValueContentString(value.Ref)

		return strings.Split(fieldsStr, " ")
	}

	return nil
}

func (f *RequiredFieldExtractor) primaryKeyFieldsIfObjectTypeIsEntity(objectType ast.ObjectTypeDefinition) (keyFields []string, ok bool) {
	for _, directiveRef := range objectType.Directives.Refs {
		if directiveName := f.document.DirectiveNameString(directiveRef); directiveName != sdlmerge.KeyDirectiveName {
			continue
		}

		value, exists := f.document.DirectiveArgumentValueByName(directiveRef, fieldsArgumentNameBytes)
		if !exists {
			continue
		}
		if value.Kind != ast.ValueKindString {
			continue
		}

		fieldsStr := f.document.StringValueContentString(value.Ref)

		return strings.Split(fieldsStr, " "), true
	}

	return nil, false
}
