// Package astimport can be used to import Nodes manually into an AST.
//
// This is useful when an AST should be created manually.
package astimport

import (
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// Importer imports Nodes into an existing AST.
// Always use NewImporter() to create a new Importer.
type Importer struct {
}

func (i *Importer) ImportDirective(ref int, from, to *ast.Document) int {
	return i.importDirectiveWithRename(ref, from, to, nil)
}

func (i *Importer) importDirectiveWithRename(ref int, from, to *ast.Document, rename map[string]string) int {
	name := string(from.Input.ByteSlice(from.Directives[ref].Name))
	args := i.importArgumentsWithRename(from.Directives[ref].Arguments.Refs, from, to, rename)
	return to.AddDirective(ast.Directive{
		Name:         to.Input.AppendInputString(name),
		HasArguments: len(args) != 0,
		Arguments: ast.ArgumentList{
			Refs: args,
		},
	})
}

func (i *Importer) importDirectivesWithRename(refs []int, from, to *ast.Document, rename map[string]string) []int {
	out := make([]int, len(refs))
	for j, k := range refs {
		out[j] = i.importDirectiveWithRename(k, from, to, rename)
	}
	return out
}

func (i *Importer) ImportDirectiveWithRename(ref int, renameTo string, from, to *ast.Document) int {
	args := i.ImportArguments(from.Directives[ref].Arguments.Refs, from, to)
	return to.AddDirective(ast.Directive{
		Name:         to.Input.AppendInputString(renameTo),
		HasArguments: len(args) != 0,
		Arguments: ast.ArgumentList{
			Refs: args,
		},
	})
}

func (i *Importer) ImportType(ref int, from, to *ast.Document) int {

	astType := ast.Type{
		TypeKind: from.Types[ref].TypeKind,
		OfType:   -1,
	}

	if astType.TypeKind == ast.TypeKindNamed {
		astType.Name = to.Input.AppendInputBytes(from.TypeNameBytes(ref))
	}

	if from.Types[ref].OfType != -1 {
		astType.OfType = i.ImportType(from.Types[ref].OfType, from, to)
	}

	to.Types = append(to.Types, astType)
	return len(to.Types) - 1
}

func (i *Importer) ImportTypeWithRename(ref int, from, to *ast.Document, renameTo string) int {

	astType := ast.Type{
		TypeKind: from.Types[ref].TypeKind,
		OfType:   -1,
	}

	if astType.TypeKind == ast.TypeKindNamed {
		astType.Name = to.Input.AppendInputString(renameTo)
	}

	if from.Types[ref].OfType != -1 {
		astType.OfType = i.ImportTypeWithRename(from.Types[ref].OfType, from, to, renameTo)
	}

	to.Types = append(to.Types, astType)
	return len(to.Types) - 1
}

func (i *Importer) ImportValue(fromValue ast.Value, from, to *ast.Document) (value ast.Value) {
	return i.importValueWithRename(fromValue, from, to, nil)
}

// importValueWithRename mirrors ImportValue but, for ValueKindVariable, looks
// the source variable name up in rename (a miss keeps the original name). List
// and object values recurse with the same map. A nil map yields a plain copy.
func (i *Importer) importValueWithRename(fromValue ast.Value, from, to *ast.Document, rename map[string]string) (value ast.Value) {
	value.Kind = fromValue.Kind

	switch fromValue.Kind {
	case ast.ValueKindFloat:
		value.Ref = to.ImportFloatValue(
			from.FloatValueRaw(fromValue.Ref),
			from.FloatValueIsNegative(fromValue.Ref))

	case ast.ValueKindInteger:
		value.Ref = to.ImportIntValue(
			from.IntValueRaw(fromValue.Ref),
			from.IntValueIsNegative(fromValue.Ref))

	case ast.ValueKindBoolean:
		value.Ref = fromValue.Ref

	case ast.ValueKindString:
		value.Ref = to.ImportStringValue(
			from.StringValueContentBytes(fromValue.Ref),
			from.StringValueIsBlockString(fromValue.Ref))

	case ast.ValueKindNull:
		// empty case

	case ast.ValueKindEnum:
		value.Ref = to.ImportEnumValue(from.EnumValueNameBytes(fromValue.Ref))

	case ast.ValueKindVariable:
		name := from.VariableValueNameString(fromValue.Ref)
		if rename != nil {
			if renamed, ok := rename[name]; ok {
				name = renamed
			}
		}
		value.Ref = to.ImportVariableValue([]byte(name))

	case ast.ValueKindList:
		value.Ref = to.ImportListValue(i.importListValuesWithRename(fromValue.Ref, from, to, rename))

	case ast.ValueKindObject:
		value.Ref = to.ImportObjectValue(i.importObjectFieldsWithRename(fromValue.Ref, from, to, rename))

	default:
		value.Kind = ast.ValueKindUnknown
		fmt.Printf("astimport.Importer.ImportValue: not implemented for ValueKind: %s\n", fromValue.Kind)
	}
	return
}

func (i *Importer) ImportObjectFields(ref int, from, to *ast.Document) (refs []int) {
	return i.importObjectFieldsWithRename(ref, from, to, nil)
}

func (i *Importer) importObjectFieldsWithRename(ref int, from, to *ast.Document, rename map[string]string) (refs []int) {
	objValue := from.ObjectValues[ref]

	for _, fieldRef := range objValue.Refs {
		objectField := from.ObjectFields[fieldRef]

		refs = append(refs, to.ImportObjectField(
			from.ObjectFieldNameBytes(fieldRef),
			i.importValueWithRename(objectField.Value, from, to, rename)))
	}
	return
}

func (i *Importer) ImportListValues(ref int, from, to *ast.Document) (refs []int) {
	return i.importListValuesWithRename(ref, from, to, nil)
}

func (i *Importer) importListValuesWithRename(ref int, from, to *ast.Document, rename map[string]string) (refs []int) {
	listValue := from.ListValues[ref]

	for _, valueRef := range listValue.Refs {
		value := i.importValueWithRename(from.Values[valueRef], from, to, rename)
		refs = append(refs, to.AddValue(value))
	}
	return
}

func (i *Importer) ImportArgument(ref int, from, to *ast.Document) int {
	return i.importArgumentWithRename(ref, from, to, nil)
}

func (i *Importer) importArgumentWithRename(ref int, from, to *ast.Document, rename map[string]string) int {
	arg := ast.Argument{
		Name:  to.Input.AppendInputBytes(from.ArgumentNameBytes(ref)),
		Value: i.importValueWithRename(from.ArgumentValue(ref), from, to, rename),
	}
	to.Arguments = append(to.Arguments, arg)
	return len(to.Arguments) - 1
}

func (i *Importer) ImportArguments(refs []int, from, to *ast.Document) []int {
	return i.importArgumentsWithRename(refs, from, to, nil)
}

func (i *Importer) importArgumentsWithRename(refs []int, from, to *ast.Document, rename map[string]string) []int {
	args := make([]int, len(refs))
	for j, k := range refs {
		args[j] = i.importArgumentWithRename(k, from, to, rename)
	}
	return args
}

func (i *Importer) ImportVariableDefinition(ref int, from, to *ast.Document) int {

	variableDefinition := ast.VariableDefinition{
		Description:   i.ImportDescription(from.VariableDefinitions[ref].Description, from, to),
		VariableValue: i.ImportValue(from.VariableDefinitions[ref].VariableValue, from, to),
		Type:          i.ImportType(from.VariableDefinitions[ref].Type, from, to),
		DefaultValue: ast.DefaultValue{
			IsDefined: from.VariableDefinitions[ref].DefaultValue.IsDefined,
		},
		// HasDirectives: false, //TODO: implement import directives
		// Directives:    ast.DirectiveList{},
	}

	if from.VariableDefinitions[ref].DefaultValue.IsDefined {
		variableDefinition.DefaultValue.Value = i.ImportValue(from.VariableDefinitions[ref].DefaultValue.Value, from, to)
	}

	to.VariableDefinitions = append(to.VariableDefinitions, variableDefinition)
	return len(to.VariableDefinitions) - 1
}

func (i *Importer) ImportVariableDefinitionWithRename(ref int, from, to *ast.Document, renameTo string) int {

	variableDefinition := ast.VariableDefinition{
		Description:   i.ImportDescription(from.VariableDefinitions[ref].Description, from, to),
		VariableValue: i.ImportValue(from.VariableDefinitions[ref].VariableValue, from, to),
		Type:          i.ImportTypeWithRename(from.VariableDefinitions[ref].Type, from, to, renameTo),
		DefaultValue: ast.DefaultValue{
			IsDefined: from.VariableDefinitions[ref].DefaultValue.IsDefined,
		},
		// HasDirectives: false, //TODO: implement import directives
		// Directives:    ast.DirectiveList{},
	}

	if from.VariableDefinitions[ref].DefaultValue.IsDefined {
		variableDefinition.DefaultValue.Value = i.ImportValue(from.VariableDefinitions[ref].DefaultValue.Value, from, to)
	}

	to.VariableDefinitions = append(to.VariableDefinitions, variableDefinition)
	return len(to.VariableDefinitions) - 1
}

// ImportVariableDefinitionWithVariableNameRename imports a variable definition
// while writing newName as the variable value name. The type is imported
// unchanged (unlike ImportVariableDefinitionWithRename, which renames the type).
func (i *Importer) ImportVariableDefinitionWithVariableNameRename(ref int, from, to *ast.Document, newName string) int {

	variableDefinition := ast.VariableDefinition{
		Description: i.ImportDescription(from.VariableDefinitions[ref].Description, from, to),
		VariableValue: ast.Value{
			Kind: ast.ValueKindVariable,
			Ref:  to.ImportVariableValue([]byte(newName)),
		},
		Type: i.ImportType(from.VariableDefinitions[ref].Type, from, to),
		DefaultValue: ast.DefaultValue{
			IsDefined: from.VariableDefinitions[ref].DefaultValue.IsDefined,
		},
	}

	if from.VariableDefinitions[ref].DefaultValue.IsDefined {
		variableDefinition.DefaultValue.Value = i.ImportValue(from.VariableDefinitions[ref].DefaultValue.Value, from, to)
	}

	to.VariableDefinitions = append(to.VariableDefinitions, variableDefinition)
	return len(to.VariableDefinitions) - 1
}

// ImportDescription copies a description from one document into another while
// preserving the original byte content, the block-string flag, and whether the
// description was defined. It does not go through Document.ImportDescription
// because that helper takes a plain string and guesses the block-string flag
// from the text, which would silently change a single-line block string like
// """foo""" into a regular "foo" on its way across.
func (i *Importer) ImportDescription(description ast.Description, from, to *ast.Document) ast.Description {
	if !description.IsDefined {
		return ast.Description{}
	}
	contentBytes := from.Input.ByteSlice(description.Content)
	return ast.Description{
		IsDefined:     true,
		IsBlockString: description.IsBlockString,
		Content:       to.Input.AppendInputBytes(contentBytes),
		Position:      description.Position,
	}
}

func (i *Importer) ImportVariableDefinitions(refs []int, from, to *ast.Document) []int {
	definitions := make([]int, len(refs))
	for j, k := range refs {
		definitions[j] = i.ImportVariableDefinition(k, from, to)
	}
	return definitions
}

func (i *Importer) ImportField(ref int, from, to *ast.Document) int {
	field := ast.Field{
		Alias: ast.Alias{
			IsDefined: from.FieldAliasIsDefined(ref),
		},
		Name:         to.Input.AppendInputBytes(from.FieldNameBytes(ref)),
		HasArguments: from.FieldHasArguments(ref),
		// HasDirectives: from.FieldHasDirectives(ref), // HasDirectives: false, //TODO: implement import directives
		SelectionSet:  -1,
		HasSelections: false,
	}
	if field.Alias.IsDefined {
		field.Alias.Name = to.Input.AppendInputBytes(from.FieldAliasBytes(ref))
	}
	if field.HasArguments {
		field.Arguments.Refs = i.ImportArguments(from.FieldArguments(ref), from, to)
	}
	to.Fields = append(to.Fields, field)
	return len(to.Fields) - 1
}

// ImportSelectionSet deep-copies a selection set across documents. It is a thin
// wrapper over ImportSelectionSetWithVariableRename with a nil rename map.
func (i *Importer) ImportSelectionSet(ref int, from, to *ast.Document) (int, error) {
	return i.ImportSelectionSetWithVariableRename(ref, from, to, nil)
}

// ImportSelectionSetWithVariableRename deep-copies a selection set across
// documents, applying rename to every variable occurrence in argument and
// directive values (a nil map keeps names verbatim). Fields carry name, alias,
// arguments, directives and nested sets; inline fragments carry the type
// condition, directives and nested sets. Fragment spreads are unsupported and
// return an error.
func (i *Importer) ImportSelectionSetWithVariableRename(ref int, from, to *ast.Document, rename map[string]string) (int, error) {
	refs := make([]int, 0, len(from.SelectionSets[ref].SelectionRefs))
	for _, selectionRef := range from.SelectionSets[ref].SelectionRefs {
		imported, err := i.importSelection(selectionRef, from, to, rename)
		if err != nil {
			return -1, err
		}
		refs = append(refs, imported)
	}
	return to.AddSelectionSetToDocument(ast.SelectionSet{SelectionRefs: refs}), nil
}

func (i *Importer) importSelection(ref int, from, to *ast.Document, rename map[string]string) (int, error) {
	switch from.Selections[ref].Kind {
	case ast.SelectionKindField:
		fieldRef, err := i.importFieldWithRename(from.Selections[ref].Ref, from, to, rename)
		if err != nil {
			return -1, err
		}
		return to.AddSelectionToDocument(ast.Selection{Kind: ast.SelectionKindField, Ref: fieldRef}), nil
	case ast.SelectionKindInlineFragment:
		fragmentRef, err := i.importInlineFragmentWithRename(from.Selections[ref].Ref, from, to, rename)
		if err != nil {
			return -1, err
		}
		return to.AddSelectionToDocument(ast.Selection{Kind: ast.SelectionKindInlineFragment, Ref: fragmentRef}), nil
	case ast.SelectionKindFragmentSpread:
		return -1, fmt.Errorf("astimport: fragment spreads are not supported")
	default:
		return -1, fmt.Errorf("astimport: unknown selection kind %v", from.Selections[ref].Kind)
	}
}

func (i *Importer) importFieldWithRename(ref int, from, to *ast.Document, rename map[string]string) (int, error) {
	var arguments ast.ArgumentList
	var directives ast.DirectiveList
	selectionSet := -1
	hasSelections := from.Fields[ref].HasSelections

	if from.FieldHasArguments(ref) {
		arguments.Refs = i.importArgumentsWithRename(from.FieldArguments(ref), from, to, rename)
	}
	if from.FieldHasDirectives(ref) {
		directives.Refs = i.importDirectivesWithRename(from.Fields[ref].Directives.Refs, from, to, rename)
	}
	if hasSelections {
		set, err := i.ImportSelectionSetWithVariableRename(from.Fields[ref].SelectionSet, from, to, rename)
		if err != nil {
			return -1, err
		}
		selectionSet = set
	}

	field := ast.Field{
		Name:          to.Input.AppendInputBytes(from.FieldNameBytes(ref)),
		Alias:         ast.Alias{IsDefined: from.FieldAliasIsDefined(ref)},
		HasArguments:  from.FieldHasArguments(ref),
		Arguments:     arguments,
		HasDirectives: from.FieldHasDirectives(ref),
		Directives:    directives,
		HasSelections: hasSelections,
		SelectionSet:  selectionSet,
	}
	if field.Alias.IsDefined {
		field.Alias.Name = to.Input.AppendInputBytes(from.FieldAliasBytes(ref))
	}
	return to.AddField(field).Ref, nil
}

func (i *Importer) importInlineFragmentWithRename(ref int, from, to *ast.Document, rename map[string]string) (int, error) {
	var directives ast.DirectiveList
	selectionSet := -1
	hasSelections := from.InlineFragments[ref].HasSelections
	typeCondition := ast.TypeCondition{Type: -1}

	if from.InlineFragmentHasTypeCondition(ref) {
		typeCondition.Type = i.ImportType(from.InlineFragments[ref].TypeCondition.Type, from, to)
	}
	if from.InlineFragments[ref].HasDirectives {
		directives.Refs = i.importDirectivesWithRename(from.InlineFragments[ref].Directives.Refs, from, to, rename)
	}
	if hasSelections {
		set, err := i.ImportSelectionSetWithVariableRename(from.InlineFragments[ref].SelectionSet, from, to, rename)
		if err != nil {
			return -1, err
		}
		selectionSet = set
	}

	fragment := ast.InlineFragment{
		TypeCondition: typeCondition,
		HasDirectives: from.InlineFragments[ref].HasDirectives,
		Directives:    directives,
		HasSelections: hasSelections,
		SelectionSet:  selectionSet,
	}
	return to.AddInlineFragment(fragment), nil
}
