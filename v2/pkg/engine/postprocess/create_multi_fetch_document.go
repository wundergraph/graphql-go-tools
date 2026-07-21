package postprocess

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astimport"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// buildMergedOperation merges the group members' stored documents into one
// aliased, @include-guarded operation and returns the compact and pretty
// printed forms.
func buildMergedOperation(members []*resolve.SingleFetch) (compact string, pretty string, err error) {
	merged := ast.NewSmallDocument()
	opSetRef := merged.AddSelectionSet().Ref
	opRef := merged.AddOperationDefinitionToRootNodes(ast.OperationDefinition{
		OperationType: ast.OperationTypeQuery,
		HasSelections: true,
		SelectionSet:  opSetRef,
	}).Ref

	if name := mergedOperationName(members); name != "" {
		merged.OperationDefinitions[opRef].Name = merged.Input.AppendInputString(name)
	}

	importer := &astimport.Importer{}
	for i, member := range members {
		kStr := strconv.Itoa(i + 1)
		op := member.MergeableOperation
		if op == nil || op.Document == nil {
			return "", "", fmt.Errorf("createMultiFetch: member %d has no document", i+1)
		}
		doc := op.Document

		// Rename every variable name to name_f<k> over the union of the document's
		// variable definitions and the recorded fragment names (which can include
		// stale keys absent from the document).
		rename := make(map[string]string, len(op.Variables))
		for _, defRef := range doc.OperationDefinitions[0].VariableDefinitions.Refs {
			origName := doc.VariableDefinitionNameString(defRef)
			rename[origName] = origName + "_f" + kStr
		}
		for _, fragment := range op.Variables {
			rename[fragment.Name] = fragment.Name + "_f" + kStr
		}

		for _, defRef := range doc.OperationDefinitions[0].VariableDefinitions.Refs {
			origName := doc.VariableDefinitionNameString(defRef)
			importedRef := importer.ImportVariableDefinitionWithVariableNameRename(defRef, doc, merged, rename[origName])
			merged.AddImportedVariableDefinitionToOperationDefinition(opRef, importedRef)
		}

		boolType := merged.AddNamedType([]byte("Boolean"))
		nonNullBool := merged.AddNonNullType(boolType)
		includeVar := merged.ImportVariableValue([]byte("includeF" + kStr))
		merged.AddVariableDefinitionToOperationDefinition(opRef, includeVar, nonNullBool)

		importedSetRef, err := importer.ImportSelectionSetWithVariableRename(doc.OperationDefinitions[0].SelectionSet, doc, merged, rename)
		if err != nil {
			return "", "", err
		}
		selectionRefs := merged.SelectionSets[importedSetRef].SelectionRefs
		if len(selectionRefs) != 1 || merged.Selections[selectionRefs[0]].Kind != ast.SelectionKindField {
			return "", "", fmt.Errorf("createMultiFetch: member %d root selection is not a single field", i+1)
		}
		fieldRef := merged.Selections[selectionRefs[0]].Ref
		if merged.FieldNameString(fieldRef) != "_entities" {
			return "", "", fmt.Errorf("createMultiFetch: member %d root field is not _entities", i+1)
		}

		merged.Fields[fieldRef].Alias = ast.Alias{IsDefined: true, Name: merged.Input.AppendInputString("f" + kStr)}
		includeArgValue := ast.Value{Kind: ast.ValueKindVariable, Ref: merged.AddVariableValue(ast.VariableValue{Name: merged.Input.AppendInputString("includeF" + kStr)})}
		includeArg := merged.AddArgument(ast.Argument{Name: merged.Input.AppendInputString("if"), Value: includeArgValue})
		directiveRef := merged.AddDirective(ast.Directive{Name: merged.Input.AppendInputString("include"), HasArguments: true, Arguments: ast.ArgumentList{Refs: []int{includeArg}}})
		merged.Fields[fieldRef].HasDirectives = true
		merged.Fields[fieldRef].Directives.Refs = append(merged.Fields[fieldRef].Directives.Refs, directiveRef)

		merged.AddSelection(opSetRef, ast.Selection{Kind: ast.SelectionKindField, Ref: fieldRef})
	}

	compact, err = astprinter.PrintString(merged)
	if err != nil {
		return "", "", err
	}
	pretty, err = astprinter.PrintStringIndent(merged, "  ")
	if err != nil {
		return "", "", err
	}
	return compact, pretty, nil
}

// mergedOperationName returns <OperationName>__multi_<id1>_<id2>... when every
// member shares the same non-empty OperationName; otherwise the empty string
// (anonymous operation).
func mergedOperationName(members []*resolve.SingleFetch) string {
	name := members[0].OperationName
	if name == "" {
		return ""
	}
	for _, member := range members[1:] {
		if member.OperationName != name {
			return ""
		}
	}
	var b strings.Builder
	b.WriteString(name)
	b.WriteString("__multi")
	for _, member := range members {
		b.WriteString("_")
		b.WriteString(strconv.Itoa(member.FetchDependencies.FetchID))
	}
	return b.String()
}
