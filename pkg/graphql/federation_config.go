package graphql

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
)

const (
	keyDirectiveName      = "key"
	requireDirectiveName  = "requires"
	externalDirectiveName = "external"
)

func NewFederationEngineConfigV2Factory(httpClient *http.Client, rawBaseSchema string, SDLs ...string) *FederationEngineConfigV2Factory {
	return &FederationEngineConfigV2Factory{
		httpClient:    httpClient,
		rawBaseSchema: rawBaseSchema,
		SDLs:          SDLs,
	}
}

type FederationEngineConfigV2Factory struct {
	rawBaseSchema string
	schema        *Schema
	httpClient    *http.Client
	SDLs          []string
}

func (f *FederationEngineConfigV2Factory) New() (*EngineV2Configuration, error) {
	var err error
	if f.schema, err = NewSchemaFromString(f.rawBaseSchema); err != nil {
		return nil, err
	}

	conf := NewEngineV2Configuration(f.schema)

	fieldConfigs, err := f.engineConfigFieldConfigs()
	if err != nil {
		return nil, err
	}

	conf.SetFieldConfigurations(fieldConfigs)

	return nil, nil
}

func (f *FederationEngineConfigV2Factory) engineConfigFieldConfigs() (plan.FieldConfigurations, error) {
	var planFieldConfigs plan.FieldConfigurations

	for _, SDL := range f.SDLs {
		doc, report := astparser.ParseGraphqlDocumentString(SDL)
		if report.HasErrors() {
			return nil, fmt.Errorf("parse graphql document string: %w", report)
		}
		extractor := &federationSDLRequiredFieldExtractor{document: &doc}
		planFieldConfigs = append(planFieldConfigs, extractor.getAllFieldRequires()...)
	}

	generatedArgs := f.schema.GetAllFieldArguments(NewSkipReservedNamesFunc())
	generatedArgsAsLookupMap := CreateTypeFieldArgumentsLookupMap(generatedArgs)
	f.engineConfigArguments(&planFieldConfigs, generatedArgsAsLookupMap)

	return planFieldConfigs, nil
}

func (f *FederationEngineConfigV2Factory) engineConfigArguments(fieldConfs *plan.FieldConfigurations, generatedArgs map[TypeFieldLookupKey]TypeFieldArguments) {
	for i := range *fieldConfs {
		if len(generatedArgs) == 0 {
			return
		}

		lookupKey := CreateTypeFieldLookupKey((*fieldConfs)[i].TypeName, (*fieldConfs)[i].FieldName)
		currentArgs, ok := generatedArgs[lookupKey]
		if !ok {
			continue
		}

		(*fieldConfs)[i].Arguments = f.createArgumentConfigurationsForArgumentNames(currentArgs.ArgumentNames)
		delete(generatedArgs, lookupKey)
	}

	for _, genArgs := range generatedArgs {
		*fieldConfs = append(*fieldConfs, plan.FieldConfiguration{
			TypeName:  genArgs.TypeName,
			FieldName: genArgs.FieldName,
			Arguments: f.createArgumentConfigurationsForArgumentNames(genArgs.ArgumentNames),
		})
	}
}

func (f *FederationEngineConfigV2Factory) createArgumentConfigurationsForArgumentNames(argumentNames []string) plan.ArgumentsConfigurations {
	argConfs := plan.ArgumentsConfigurations{}
	for _, argName := range argumentNames {
		argConf := plan.ArgumentConfiguration{
			Name:       argName,
			SourceType: plan.FieldArgumentSource,
		}

		argConfs = append(argConfs, argConf)
	}

	return argConfs
}

// federationSDLRequiredFieldExtractor
type federationSDLRequiredFieldExtractor struct {
	document *ast.Document
}

func (f *federationSDLRequiredFieldExtractor) getAllFieldRequires() plan.FieldConfigurations {
	var fieldRequires plan.FieldConfigurations

	f.addFieldsForObjectExtensionDefinitions(&fieldRequires)
	f.addFieldsForObjectDefinitions(&fieldRequires)

	return fieldRequires
}

func (f *federationSDLRequiredFieldExtractor) addFieldsForObjectExtensionDefinitions(fieldRequires *plan.FieldConfigurations) {
	for _, objectTypeExt := range f.document.ObjectTypeExtensions {
		objectType := objectTypeExt.ObjectTypeDefinition
		typeName := f.document.Input.ByteSliceString(objectType.Name)

		primaryKeys, ok := f.primaryKeyFieldsIfObjectTypeIsEntity(objectType)
		if !ok {
			continue
		}

		for _, fieldRef := range objectType.FieldsDefinition.Refs {
			if isFederationExternalField(f.document, fieldRef) {
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

func (f *federationSDLRequiredFieldExtractor) addFieldsForObjectDefinitions(fieldRequires *plan.FieldConfigurations) {
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

func (f *federationSDLRequiredFieldExtractor) requiredFieldsByRequiresDirective(ref int) []string {
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

func (f *federationSDLRequiredFieldExtractor) primaryKeyFieldsIfObjectTypeIsEntity(objectType ast.ObjectTypeDefinition) (keyFields []string, ok bool) {
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

// Extract all fields from Entities.
// If the Entity is extended - only local fields (fields without external directive).

type federationRootNodeExtractor struct {
	document *ast.Document
}

func NewRootNodeExtractor(document *ast.Document) *federationRootNodeExtractor {
	return &federationRootNodeExtractor{document: document}
}

func (r *federationRootNodeExtractor) GetAllRootNodes() []plan.TypeField {
	var rootNodes []plan.TypeField

	for _, objectTypeExt := range r.document.ObjectTypeExtensions {
		r.addRootNodesForObjectDefinition(objectTypeExt.ObjectTypeDefinition, &rootNodes)
	}

	for _, objectType := range r.document.ObjectTypeDefinitions {
		r.addRootNodesForObjectDefinition(objectType, &rootNodes)
	}

	return rootNodes
}

func (r *federationRootNodeExtractor) addRootNodesForObjectDefinition(objectType ast.ObjectTypeDefinition, rootNodes *[]plan.TypeField) {
	typeName := r.document.Input.ByteSliceString(objectType.Name)

	if !isFederationEntity(r.document, objectType) && !r.isRootOperationTypeName(typeName) {
		return
	}

	var fieldNames []string

	for _, fieldRef := range objectType.FieldsDefinition.Refs {
		if isFederationExternalField(r.document, fieldRef) {
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

func (r *federationRootNodeExtractor) isRootOperationTypeName(typeName string) bool {
	rootOperationNames := map[string]struct{}{
		"Query":        {},
		"Mutation":     {},
		"Subscription": {},
	}

	_, ok := rootOperationNames[typeName]

	return ok
}

//

// utils
func isFederationExternalField(document *ast.Document, ref int) bool {
	for _, directiveRef := range document.FieldDefinitions[ref].Directives.Refs {
		if directiveName := document.DirectiveNameString(directiveRef); directiveName == externalDirectiveName {
			return true
		}
	}

	return false
}

func isFederationEntity(document *ast.Document, objectType ast.ObjectTypeDefinition) bool {
	for _, directiveRef := range objectType.Directives.Refs {
		if directiveName := document.DirectiveNameString(directiveRef); directiveName == keyDirectiveName {
			return true
		}
	}

	return false
}
