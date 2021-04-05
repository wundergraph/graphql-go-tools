package planconfiguration

import (
	"strings"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
)

// Currently it doesnt support multiple primary keys https://www.apollographql.com/docs/federation/entities/#defining-multiple-primary-keys
// and nested fields in primary keys https://www.apollographql.com/docs/federation/entities/#defining-a-compound-primary-key

const (
	keyDirectiveName      = "key"
	requireDirectiveName  = "requires"
	externalDirectiveName = "external"
)

type TypeFieldRequires struct {
	TypeName       string
	FieldName      string
	RequiresFields []string
}

type RequiredFieldExtractor struct {
	document *ast.Document
}

func NewRequiredFieldExtractor(document *ast.Document) *RequiredFieldExtractor {
	return &RequiredFieldExtractor{document: document}
}

func (r *RequiredFieldExtractor) GetAllFieldRequires() []TypeFieldRequires {
	typeFieldRequires := make([]TypeFieldRequires, 0)

	r.addFieldsForObjectExtensionDefinitions(&typeFieldRequires)
	r.addFieldsForObjectDefinitions(&typeFieldRequires)

	return typeFieldRequires
}

func (r *RequiredFieldExtractor) addFieldsForObjectExtensionDefinitions(fieldRequires *[]TypeFieldRequires) {
	for _, objectTypeExt := range r.document.ObjectTypeExtensions {
		objectType := objectTypeExt.ObjectTypeDefinition
		typeName := r.document.Input.ByteSliceString(objectType.Name)

		primaryKeys, ok := r.primaryKeyFieldsIfObjectTypeIsEntity(objectType)
		if !ok {
			continue
		}

		for _, fieldRef := range objectType.FieldsDefinition.Refs {
			if !r.isExternalField(fieldRef) {
				continue
			}

			fieldName := r.document.FieldDefinitionNameString(fieldRef)

			requiredFields := make([]string, len(primaryKeys))
			copy(requiredFields, primaryKeys)

			requiredFieldsByRequiresDirective := r.requiredFieldsByRequiresDirective(fieldRef)
			requiredFields = append(requiredFields, requiredFieldsByRequiresDirective...)

			*fieldRequires = append(*fieldRequires, TypeFieldRequires{
				TypeName:       typeName,
				FieldName:      fieldName,
				RequiresFields: requiredFields,
			})
		}
	}
}

func (r *RequiredFieldExtractor) addFieldsForObjectDefinitions(fieldRequires *[]TypeFieldRequires) {
	for _, objectType := range r.document.ObjectTypeDefinitions {
		typeName := r.document.Input.ByteSliceString(objectType.Name)

		primaryKeys, ok := r.primaryKeyFieldsIfObjectTypeIsEntity(objectType)
		if !ok {
			continue
		}

		primaryKeysSet := make(map[string]struct{}, len(primaryKeys))
		for _, val := range primaryKeys {
			primaryKeysSet[val] = struct{}{}
		}

		for _, fieldRef := range objectType.FieldsDefinition.Refs {
			fieldName := r.document.FieldDefinitionNameString(fieldRef)
			if _, ok := primaryKeysSet[fieldName]; ok { // Field is part of primary key, it couldn't have any required fields
				continue
			}

			requiredFields := make([]string, len(primaryKeys))
			copy(requiredFields, primaryKeys)

			*fieldRequires = append(*fieldRequires, TypeFieldRequires{
				TypeName:       typeName,
				FieldName:      fieldName,
				RequiresFields: requiredFields,
			})
		}
	}
}

func (r *RequiredFieldExtractor) primaryKeyFieldsIfObjectTypeIsEntity(objectType ast.ObjectTypeDefinition) (keyFields []string, ok bool) {
	for _, directiveRef := range objectType.Directives.Refs {
		if directiveName := r.document.DirectiveNameString(directiveRef); directiveName != keyDirectiveName {
			continue
		}

		value, exists := r.document.DirectiveArgumentValueByName(directiveRef, []byte("fields"))
		if !exists {
			continue
		}
		if value.Kind != ast.ValueKindString {
			continue
		}

		fieldsStr := r.document.StringValueContentString(value.Ref)

		return strings.Split(fieldsStr, " "), true
	}

	return nil, false
}

func (r *RequiredFieldExtractor) requiredFieldsByRequiresDirective(ref int) []string {
	for _, directiveRef := range r.document.FieldDefinitions[ref].Directives.Refs {
		if directiveName := r.document.DirectiveNameString(directiveRef); directiveName != requireDirectiveName {
			continue
		}

		value, exists := r.document.DirectiveArgumentValueByName(directiveRef, []byte("fields"))
		if !exists {
			continue
		}
		if value.Kind != ast.ValueKindString {
			continue
		}

		fieldsStr := r.document.StringValueContentString(value.Ref)

		return strings.Split(fieldsStr, " ")
	}

	return nil
}

func (r *RequiredFieldExtractor) isExternalField(ref int) bool {
	for _, directiveRef := range r.document.FieldDefinitions[ref].Directives.Refs {
		if directiveName := r.document.DirectiveNameString(directiveRef); directiveName != externalDirectiveName {
			return true
		}
	}

	return false
}
