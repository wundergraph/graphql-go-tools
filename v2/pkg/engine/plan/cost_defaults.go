package plan

import (
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// fillListSizeDefaults fills FieldListSize.SlicingArgumentDefaults by
// walking the data source's parsed upstream SDL.
//
// Composition has already validated the @listSize directive; the engine acts as a
// pure extractor here. Anything that does not resolve to an Int default is silently
// skipped — composition is the validator.
func fillListSizeDefaults(listSizes map[FieldCoordinate]*FieldListSize, schema *ast.Document) {
	if schema == nil || len(listSizes) == 0 {
		return
	}
	for coords, listSize := range listSizes {
		if listSize == nil || len(listSize.SlicingArguments) == 0 {
			continue
		}

		typeNode, exists := schema.Index.FirstNodeByNameStr(coords.TypeName)
		if !exists {
			continue
		}
		fieldDefRef, exists := schema.NodeFieldDefinitionByName(typeNode, []byte(coords.FieldName))
		if !exists {
			continue
		}
		if !schema.FieldDefinitionHasArgumentsDefinitions(fieldDefRef) {
			continue
		}

		for _, slicingArgPath := range listSize.SlicingArguments {
			segments := strings.Split(slicingArgPath, ".")
			value, ok := getEffectiveSlicingArgLeafDefault(schema, fieldDefRef, segments)
			if !ok {
				continue
			}
			if listSize.SlicingArgumentDefaults == nil {
				listSize.SlicingArgumentDefaults = make(map[string]int)
			}
			listSize.SlicingArgumentDefaults[slicingArgPath] = value
		}
	}
}

// getEffectiveSlicingArgLeafDefault returns the effective leaf Int default for a
// slicing argument path declared on the given field definition, or (0, false) if
// no default along the chain resolves to a defined Int leaf.
func getEffectiveSlicingArgLeafDefault(schema *ast.Document, fieldDefRef int, segments []string) (int, bool) {
	if len(segments) == 0 {
		return 0, false
	}

	// Build the chain of InputValueDefinition refs. One per path segment,
	// starting with the field's argument matching segments[0] and traversing through
	// nested input-object field definitions for the rest.
	// If any segment fails to resolve, we bail.
	chain := make([]int, 0, len(segments))

	// Segment 0: the field's argument.
	argRefs := schema.FieldDefinitionArgumentsDefinitions(fieldDefRef)
	firstArgRef := -1
	for _, ref := range argRefs {
		if schema.InputValueDefinitionNameString(ref) == segments[0] {
			firstArgRef = ref
			break
		}
	}
	if firstArgRef == -1 {
		return 0, false
	}
	chain = append(chain, firstArgRef)

	// Segments 1...n-1. Descend through nested input-object field definitions.
	currentTypeRef := schema.InputValueDefinitionType(firstArgRef)
	for i := 1; i < len(segments); i++ {
		typeName := schema.ResolveTypeNameString(currentTypeRef)
		typeNode, exists := schema.Index.FirstNodeByNameStr(typeName)
		if !exists || typeNode.Kind != ast.NodeKindInputObjectTypeDefinition {
			return 0, false
		}
		nextRef := schema.InputObjectTypeDefinitionInputValueDefinitionByName(typeNode.Ref, []byte(segments[i]))
		if nextRef == -1 {
			return 0, false
		}
		chain = append(chain, nextRef)
		currentTypeRef = schema.InputValueDefinitionType(nextRef)
	}

	// Walk the chain outermost-to-leaf: for each position i, take that
	// InputValueDefinition's declared default and step through segments[i+1...n-1]
	// inside the default's object-literal AST. The first chain position whose
	// default resolves to a defined ValueKindInteger leaf wins.
	// A defined-but-non-Int outer value shadows inner defaults.
	for i := 0; i < len(chain); i++ {
		if !schema.InputValueDefinitionHasDefaultValue(chain[i]) {
			continue
		}
		value := schema.InputValueDefinitionDefaultValue(chain[i])

		// Scan segments[i+1...n-1] for the default's object literal.
		resolved := value
		ok := true
		for j := i + 1; j < len(segments); j++ {
			if resolved.Kind != ast.ValueKindObject {
				ok = false
				break
			}
			fieldValue, found := findObjectFieldValue(schema, resolved.Ref, segments[j])
			if !found {
				ok = false
				break
			}
			resolved = fieldValue
		}
		if !ok {
			continue // This position's default doesn't cover the rest of the path.
		}

		// resolved is the candidate leaf.
		// If it is not an Int, this outer default shadows inner defaults.
		if resolved.Kind == ast.ValueKindInteger {
			return int(schema.IntValueAsInt(resolved.Ref)), true
		}
		return 0, false
	}
	return 0, false
}

// findObjectFieldValue looks up a named field inside an object-literal Value (the
// default value of an InputValueDefinition that has Kind == ValueKindObject) and
// returns its Value plus true if found.
func findObjectFieldValue(schema *ast.Document, objectValueRef int, fieldName string) (ast.Value, bool) {
	for _, fieldRef := range schema.ObjectValues[objectValueRef].Refs {
		if schema.ObjectFieldNameString(fieldRef) == fieldName {
			return schema.ObjectFieldValue(fieldRef), true
		}
	}
	return ast.Value{}, false
}
