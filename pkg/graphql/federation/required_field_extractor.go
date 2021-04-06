package federation

import (
	"strings"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
)

// requiredFieldExtractor
type requiredFieldExtractor struct {
	document *ast.Document
}

func (f *requiredFieldExtractor) getAllFieldRequires() plan.FieldConfigurations {
	var fieldRequires plan.FieldConfigurations

	f.addFieldsForObjectExtensionDefinitions(&fieldRequires)
	f.addFieldsForObjectDefinitions(&fieldRequires)

	return fieldRequires
}

func (f *requiredFieldExtractor) addFieldsForObjectExtensionDefinitions(fieldRequires *plan.FieldConfigurations) {
	for _, objectTypeExt := range f.document.ObjectTypeExtensions {
		objectType := objectTypeExt.ObjectTypeDefinition
		typeName := f.document.Input.ByteSliceString(objectType.Name)

		primaryKeys, ok := f.primaryKeyFieldsIfObjectTypeIsEntity(objectType)
		if !ok {
			continue
		}

		for _, fieldRef := range objectType.FieldsDefinition.Refs {
			if isExternalField(f.document, fieldRef) {
				continue
			}

			fieldName := f.document.FieldDefinitionNameString(fieldRef)

			requiredFields := make([]string, len(primaryKeys))
			copy(requiredFields, primaryKeys)

			requiredFieldsByRequiresDirective := f.requiredFieldsByRequiresDirective(fieldRef)
			requiredFields = append(requiredFields, requiredFieldsByRequiresDirective...)

			*fieldRequires = append(*fieldRequires, plan.FieldConfiguration{
				TypeName:       typeName,
				FieldName:      fieldName,
				RequiresFields: requiredFields,
			})
		}
	}
}

func (f *requiredFieldExtractor) addFieldsForObjectDefinitions(fieldRequires *plan.FieldConfigurations) {
	for _, objectType := range f.document.ObjectTypeDefinitions {
		typeName := f.document.Input.ByteSliceString(objectType.Name)

		primaryKeys, ok := f.primaryKeyFieldsIfObjectTypeIsEntity(objectType)
		if !ok {
			continue
		}

		primaryKeysSet := make(map[string]struct{}, len(primaryKeys))
		for _, val := range primaryKeys {
			primaryKeysSet[val] = struct{}{}
		}

		for _, fieldRef := range objectType.FieldsDefinition.Refs {
			fieldName := f.document.FieldDefinitionNameString(fieldRef)
			if _, ok := primaryKeysSet[fieldName]; ok { // Field is part of primary key, it couldn't have any required fields
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

func (f *requiredFieldExtractor) requiredFieldsByRequiresDirective(ref int) []string {
	for _, directiveRef := range f.document.FieldDefinitions[ref].Directives.Refs {
		if directiveName := f.document.DirectiveNameString(directiveRef); directiveName != requireDirectiveName {
			continue
		}

		value, exists := f.document.DirectiveArgumentValueByName(directiveRef, []byte("fields"))
		if !exists {
			continue
		}
		if value.Kind != ast.ValueKindString {
			continue
		}

		fieldsStr := f.document.StringValueContentString(value.Ref)

		return strings.Split(fieldsStr, " ")
	}

	return nil
}

func (f *requiredFieldExtractor) primaryKeyFieldsIfObjectTypeIsEntity(objectType ast.ObjectTypeDefinition) (keyFields []string, ok bool) {
	for _, directiveRef := range objectType.Directives.Refs {
		if directiveName := f.document.DirectiveNameString(directiveRef); directiveName != keyDirectiveName {
			continue
		}

		value, exists := f.document.DirectiveArgumentValueByName(directiveRef, []byte("fields"))
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
