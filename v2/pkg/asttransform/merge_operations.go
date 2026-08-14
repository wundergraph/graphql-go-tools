package asttransform

import (
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astimport"
)

// OperationMergeMember describes one operation document to fold into a merged
// operation. Its single root selection is imported under Alias, guarded by
// @include(if: $IncludeVariable), with every variable reference and definition
// renamed through VariableRename.
type OperationMergeMember struct {
	// Document holds a single query operation definition to merge. Ownership is
	// not transferred; the document is read, never mutated.
	Document *ast.Document
	// Alias is applied to the member's root field ("f1").
	Alias string
	// IncludeVariable is the synthetic Boolean! variable name guarding the
	// member's root field ("includeF1").
	IncludeVariable string
	// VariableRename maps every original variable name (definitions and
	// references) to its merged name. Callers must supply a total mapping over
	// the member's variable definitions.
	VariableRename map[string]string
}

// MergeOperationDocuments builds a new document with a single query operation containing every
// member's root selection aliased and guarded by @include(if: $IncludeVariable).
// Variable definitions are emitted per member in document order followed by that
// member's synthetic include definition. Each member's root selection must be a
// single field; fragment spreads in member documents are rejected.
// TODO: Directives on operations are not handled (merged).
func MergeOperationDocuments(operationName string, members []OperationMergeMember) (*ast.Document, error) {
	merged := ast.NewSmallDocument()
	opSetRef := merged.AddSelectionSet().Ref
	opRef := merged.AddOperationDefinitionToRootNodes(ast.OperationDefinition{
		OperationType: ast.OperationTypeQuery,
		HasSelections: true,
		SelectionSet:  opSetRef,
	}).Ref
	if operationName != "" {
		merged.OperationDefinitions[opRef].Name = merged.Input.AppendInputString(operationName)
	}

	importer := &astimport.Importer{}
	nonNullBool := merged.AddNonNullNamedType([]byte("Boolean"))
	for i, member := range members {
		if member.Document == nil {
			return nil, fmt.Errorf("asttransform: member %d has no document", i+1)
		}
		if err := mergeVariableDefinitions(merged, opRef, importer, member, nonNullBool, i); err != nil {
			return nil, err
		}
		if err := mergeMemberSelection(merged, opSetRef, importer, member, i); err != nil {
			return nil, err
		}
	}
	return merged, nil
}

// mergeVariableDefinitions imports the member's renamed variable definitions in
// document order, then appends the synthetic non-null Boolean include definition,
// matching the emit order the printed output depends on.
// A variable definition without a rename entry is an error.
func mergeVariableDefinitions(merged *ast.Document, opRef int, importer *astimport.Importer, member OperationMergeMember, nonNullBool int, i int) error {
	doc := member.Document
	for _, defRef := range doc.OperationDefinitions[0].VariableDefinitions.Refs {
		origName := doc.VariableDefinitionNameString(defRef)
		newName, ok := member.VariableRename[origName]
		if !ok || newName == "" {
			return fmt.Errorf("asttransform: member %d has no rename for variable %q", i+1, origName)
		}
		importedRef := importer.ImportVariableDefinitionWithVariableNameRename(defRef, doc, merged, newName)
		merged.AddImportedVariableDefinitionToOperationDefinition(opRef, importedRef)
	}
	includeVar := merged.ImportVariableValue([]byte(member.IncludeVariable))
	merged.AddVariableDefinitionToOperationDefinition(opRef, includeVar, nonNullBool)
	return nil
}

// mergeMemberSelection imports the member's root selection set (renaming
// variable references), validates it is a single field, aliases it, attaches
// the @include guard, and links it into the merged operation's selection set.
func mergeMemberSelection(merged *ast.Document, opSetRef int, importer *astimport.Importer, member OperationMergeMember, i int) error {
	doc := member.Document
	importedSetRef, err := importer.ImportSelectionSetWithVariableRename(doc.OperationDefinitions[0].SelectionSet, doc, merged, member.VariableRename)
	if err != nil {
		return err
	}
	selectionRefs := merged.SelectionSets[importedSetRef].SelectionRefs
	if len(selectionRefs) != 1 || merged.Selections[selectionRefs[0]].Kind != ast.SelectionKindField {
		return fmt.Errorf("asttransform: member %d root selection is not a single field", i+1)
	}
	fieldRef := merged.Selections[selectionRefs[0]].Ref
	merged.Fields[fieldRef].Alias = ast.Alias{IsDefined: true, Name: merged.Input.AppendInputString(member.Alias)}
	attachIncludeDirective(merged, fieldRef, member.IncludeVariable)
	merged.AddSelection(opSetRef, ast.Selection{Kind: ast.SelectionKindField, Ref: fieldRef})
	return nil
}

// attachIncludeDirective appends @include(if: $includeVariable) to the field.
func attachIncludeDirective(merged *ast.Document, fieldRef int, includeVariable string) {
	includeArgValue := ast.Value{Kind: ast.ValueKindVariable, Ref: merged.AddVariableValue(ast.VariableValue{Name: merged.Input.AppendInputString(includeVariable)})}
	includeArg := merged.AddArgument(ast.Argument{Name: merged.Input.AppendInputString("if"), Value: includeArgValue})
	directiveRef := merged.AddDirective(ast.Directive{Name: merged.Input.AppendInputString("include"), HasArguments: true, Arguments: ast.ArgumentList{Refs: []int{includeArg}}})
	merged.Fields[fieldRef].HasDirectives = true
	merged.Fields[fieldRef].Directives.Refs = append(merged.Fields[fieldRef].Directives.Refs, directiveRef)
}
