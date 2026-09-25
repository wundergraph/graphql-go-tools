package postprocess

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// buildMergedOperation merges the group members' stored documents into one
// aliased, @include-guarded operation and returns the compact and pretty
// printed forms. It is the federation-specific adapter over
// asttransform.MergeOperationDocuments: it derives the rename maps and
// operation name from the resolve types, enforces the _entities root field
// constraint, and does the printing.
func buildMergedOperation(members []*resolve.SingleFetch) (compact string, pretty string, err error) {
	mergeMembers := make([]asttransform.OperationMergeMember, 0, len(members))
	for i, member := range members {
		kStr := strconv.Itoa(i + 1)
		op := member.SubgraphOperation
		if op == nil || op.Document == nil {
			return "", "", fmt.Errorf("createMultiFetch: member %d has no document", i+1)
		}
		if err := validateEntitiesRootField(op.Document, i); err != nil {
			return "", "", err
		}
		mergeMembers = append(mergeMembers, asttransform.OperationMergeMember{
			Document:        op.Document,
			Alias:           "f" + kStr,
			IncludeVariable: "includeF" + kStr,
			VariableRename:  buildVariableRename(op, kStr),
		})
	}

	merged, err := asttransform.MergeOperationDocuments(mergedOperationName(members), mergeMembers)
	if err != nil {
		return "", "", err
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

// buildVariableRename maps every variable name to name_f<k> over the union of
// the document's variable definitions and the recorded fragment names
// (which can include stale keys absent from the document).
func buildVariableRename(op *resolve.SubgraphOperation, kStr string) map[string]string {
	doc := op.Document
	rename := make(map[string]string, len(op.Variables))
	for _, defRef := range doc.OperationDefinitions[0].VariableDefinitions.Refs {
		origName := doc.VariableDefinitionNameString(defRef)
		rename[origName] = origName + "_f" + kStr
	}
	for _, fragment := range op.Variables {
		rename[fragment.Name] = fragment.Name + "_f" + kStr
	}
	return rename
}

// validateEntitiesRootField enforces the federation-specific requirement that a
// member's root selection is the single field _entities.
func validateEntitiesRootField(doc *ast.Document, i int) error {
	if len(doc.OperationDefinitions) == 0 {
		return fmt.Errorf("createMultiFetch: member %d document has no operation definition", i+1)
	}
	setRef := doc.OperationDefinitions[0].SelectionSet
	if setRef < 0 || setRef >= len(doc.SelectionSets) {
		return fmt.Errorf("createMultiFetch: member %d operation has no root selection set", i+1)
	}
	refs := doc.SelectionSets[setRef].SelectionRefs
	if len(refs) == 1 && doc.Selections[refs[0]].Kind == ast.SelectionKindField {
		if doc.FieldNameString(doc.Selections[refs[0]].Ref) == "_entities" {
			return nil
		}
	}
	return fmt.Errorf("createMultiFetch: member %d root field is not _entities", i+1)
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
