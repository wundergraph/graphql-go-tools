package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
)

// nolint: golint
func LookupObjectTypeDefinitionByName(name string, definitions ParsedDefinitions) (objectType document.ObjectTypeDefinition, found bool) {
	for _, objectType = range definitions.ObjectTypeDefinitions {
		if objectType.Name == name {
			return objectType, true
		}
	}

	return objectType, false
}

// nolint: golint
func LookupDirectivesByObjectType(definition document.ObjectTypeDefinition, definitions ParsedDefinitions) (directives []document.Directive) {

	for _, i := range definition.Directives {
		directives = append(directives, definitions.Directives[i])
	}

	return
}

// nolint: golint
func LookupInterfaceTypeDefinitionByName(name string, definitions ParsedDefinitions) (interfaceTypeDefinition document.InterfaceTypeDefinition, found bool) {
	for _, interfaceTypeDefinition = range definitions.InterfaceTypeDefinitions {
		if interfaceTypeDefinition.Name == name {
			return interfaceTypeDefinition, true
		}
	}

	return interfaceTypeDefinition, false
}

// nolint: golint
func LookupDirectivesByInterfaceType(definition document.InterfaceTypeDefinition, definitions ParsedDefinitions) (directives []document.Directive) {

	for _, i := range definition.Directives {
		directives = append(directives, definitions.Directives[i])
	}

	return
}

// nolint: golint
func LookupEnumTypeDefinitionByName(name string, definitions ParsedDefinitions) (enumTypeDefinition document.EnumTypeDefinition, found bool) {
	for _, enumTypeDefinition = range definitions.EnumTypeDefinitions {
		if enumTypeDefinition.Name == name {
			return enumTypeDefinition, true
		}
	}

	return enumTypeDefinition, false
}

// nolint: golint
func LookupDirectivesByEnumType(definition document.EnumTypeDefinition, definitions ParsedDefinitions) (directives []document.Directive) {

	for _, i := range definition.Directives {
		directives = append(directives, definitions.Directives[i])
	}

	return
}

// nolint: golint
func LookupInputTypeDefinitionByName(name string, definitions ParsedDefinitions) (inputObjectTypeDefinition document.InputObjectTypeDefinition, found bool) {
	for _, inputObjectTypeDefinition = range definitions.InputObjectTypeDefinitions {
		if inputObjectTypeDefinition.Name == name {
			return inputObjectTypeDefinition, true
		}
	}

	return inputObjectTypeDefinition, false
}

// nolint: golint
func LookupDirectivesByInputType(definition document.InputObjectTypeDefinition, definitions ParsedDefinitions) (directives []document.Directive) {

	for _, i := range definition.Directives {
		directives = append(directives, definitions.Directives[i])
	}

	return
}

// nolint: golint
func LookupDirectiveDefinitionByName(name string, definitions ParsedDefinitions) (directiveDefinition document.DirectiveDefinition, found bool) {
	for _, directiveDefinition = range definitions.DirectiveDefinitions {
		if directiveDefinition.Name == name {
			return directiveDefinition, true
		}
	}

	return directiveDefinition, false
}
