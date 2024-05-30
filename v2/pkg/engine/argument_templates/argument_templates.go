package argument_templates

import (
	"bytes"
	"fmt"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"regexp"
	"strings"
)

// ArgumentTemplateRegex dictates form {{args.nested.path}} with flexible whitespace surrounding args.path
var ArgumentTemplateRegex = regexp.MustCompile(`{{\s*args\.((?:[a-zA-Z0-9_]+\.?\b)+)\s*}}`)

// The argument template delimiter is currently a period
var argumentTemplateDelimiter = "."

// ContainsArgumentTemplateString checks whether the value contains an argument template string
// Currently partially naïve. If the need arises, the more expensive regex.find could be used.
func ContainsArgumentTemplateString(value []byte) bool {
	return bytes.Contains(value, []byte("{{"))
}

type ArgumentPathValidationResult struct {
	ArgumentPath           []string
	FinalInputValueTypeRef int
}

func inputObjectDefinitionRefByInputValueDefinitionRef(definition *ast.Document, inputValueDefinitionRef int) (inputObjectDefinitionRef int, inputValueTypeRef int) {
	inputValueTypeRef = definition.InputValueDefinitions[inputValueDefinitionRef].Type
	typeNameBytes := definition.ResolveTypeNameBytes(inputValueTypeRef)
	node, ok := definition.Index.FirstNodeByNameBytes(typeNameBytes)
	if !ok || node.Kind != ast.NodeKindInputObjectTypeDefinition {
		return ast.InvalidRef, inputValueTypeRef
	}
	return node.Ref, inputValueTypeRef
}

func ValidateArgumentPath(definition *ast.Document, group string, fieldDefinitionRef int) (*ArgumentPathValidationResult, error) {
	argumentPath := strings.Split(group, argumentTemplateDelimiter)
	argumentNameBytes := []byte(argumentPath[0])
	inputValueDefinitionRef, ok := definition.InputValueDefinitionRefByFieldDefinitionRefAndArgumentNameBytes(fieldDefinitionRef, argumentNameBytes)
	if !ok {
		return nil, fmt.Errorf(`path "%s" references undefined argument "%s"`, group, argumentNameBytes)
	}
	inputObjectDefinitionRef, lastInputValueTypeRef := inputObjectDefinitionRefByInputValueDefinitionRef(definition, inputValueDefinitionRef)
	// Validate the argument path, starting from the first field
	for _, fieldName := range argumentPath[1:] {
		inputValueNameBytes := []byte(fieldName)
		if inputObjectDefinitionRef < 0 {
			return nil, fmt.Errorf(`path "%s" references nested input field "%s" whose parent is invalid or undefined`, group, inputValueNameBytes)
		}
		inputValueDefinitionRef, ok = definition.InputValueDefinitionRefByInputObjectDefinitionRefAndFieldNameBytes(inputObjectDefinitionRef, inputValueNameBytes)
		if !ok {
			return nil, fmt.Errorf(`path "%s" references undefined nested input field "%s"`, group, inputValueNameBytes)
		}
		inputObjectDefinitionRef, lastInputValueTypeRef = inputObjectDefinitionRefByInputValueDefinitionRef(definition, inputValueDefinitionRef)
	}
	// The last segment of the path should be a leaf type; consequently, it cannot itself be a parent (Input Object type)
	if inputObjectDefinitionRef != ast.InvalidRef {
		return nil, fmt.Errorf(`path "%s" ends with non-leaf input field "%s"`, group, argumentPath[len(argumentPath)-1])
	}
	return &ArgumentPathValidationResult{
		ArgumentPath:           argumentPath,
		FinalInputValueTypeRef: lastInputValueTypeRef,
	}, nil
}
