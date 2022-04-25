package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

type removeDuplicateFieldlessValueTypesVisitor struct {
	document          *ast.Document
	valueTypeSet      map[string]FieldlessValueType
	rootNodesToRemove []ast.Node
	lastEnumRef       int
	lastUnionRef      int
	lastScalarRef     int
}

func newRemoveDuplicateFieldlessValueTypesVisitor() *removeDuplicateFieldlessValueTypesVisitor {
	return &removeDuplicateFieldlessValueTypesVisitor{
		nil,
		make(map[string]FieldlessValueType),
		nil,
		ast.InvalidRef,
		ast.InvalidRef,
		ast.InvalidRef,
	}
}

func (r *removeDuplicateFieldlessValueTypesVisitor) Register(walker *astvisitor.Walker) {
	walker.RegisterEnterDocumentVisitor(r)
	walker.RegisterEnterEnumTypeDefinitionVisitor(r)
	walker.RegisterEnterScalarTypeDefinitionVisitor(r)
	walker.RegisterEnterUnionTypeDefinitionVisitor(r)
	walker.RegisterLeaveDocumentVisitor(r)
}

func (r *removeDuplicateFieldlessValueTypesVisitor) EnterDocument(operation, _ *ast.Document) {
	r.document = operation
}

func (r *removeDuplicateFieldlessValueTypesVisitor) EnterEnumTypeDefinition(ref int) {
	if ref <= r.lastEnumRef {
		return
	}
	name := r.document.EnumTypeDefinitionNameString(ref)
	enum, exists := r.valueTypeSet[name]
	if exists {
		enum.AppendValueRefs(r.document.EnumTypeDefinitions[ref].EnumValuesDefinition.Refs)
		r.rootNodesToRemove = append(r.rootNodesToRemove, ast.Node{Kind: ast.NodeKindEnumTypeDefinition, Ref: ref})
	} else {
		r.valueTypeSet[name] = EnumValueType{&r.document.EnumTypeDefinitions[ref], name}
	}
	r.lastEnumRef = ref
}

func (r *removeDuplicateFieldlessValueTypesVisitor) EnterScalarTypeDefinition(ref int) {
	if ref <= r.lastScalarRef {
		return
	}
	name := r.document.ScalarTypeDefinitionNameString(ref)
	_, exists := r.valueTypeSet[name]
	if exists {
		r.rootNodesToRemove = append(r.rootNodesToRemove, ast.Node{Kind: ast.NodeKindScalarTypeDefinition, Ref: ref})
	} else {
		r.valueTypeSet[name] = ScalarValueType{name}
	}
	r.lastScalarRef = ref
}

func (r *removeDuplicateFieldlessValueTypesVisitor) EnterUnionTypeDefinition(ref int) {
	if ref <= r.lastUnionRef {
		return
	}
	name := r.document.UnionTypeDefinitionNameString(ref)
	union, exists := r.valueTypeSet[name]
	if exists {
		union.AppendValueRefs(r.document.UnionTypeDefinitions[ref].UnionMemberTypes.Refs)
		r.rootNodesToRemove = append(r.rootNodesToRemove, ast.Node{Kind: ast.NodeKindUnionTypeDefinition, Ref: ref})
	} else {
		r.valueTypeSet[name] = UnionValueType{&r.document.UnionTypeDefinitions[ref], name}
	}
	r.lastUnionRef = ref
}

func (r *removeDuplicateFieldlessValueTypesVisitor) LeaveDocument(_, _ *ast.Document) {
	if r.rootNodesToRemove == nil {
		return
	}
	for _, valueType := range r.valueTypeSet {
		if _, ok := valueType.(ScalarValueType); ok {
			continue
		}
		valueSet := make(map[string]bool)
		var refsToKeep []int
		for _, ref := range valueType.ValueRefs() {
			name := valueType.ValueName(r, ref)
			if !valueSet[name] {
				valueSet[name] = true
				refsToKeep = append(refsToKeep, ref)
			}
		}
		valueType.SetValueRefs(refsToKeep)
	}
	r.document.DeleteRootNodesInSingleLoop(r.rootNodesToRemove)
}
