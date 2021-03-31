package federation

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"io"
)

type printingVisitor struct {
	*astvisitor.Walker
	out                   io.Writer
	operation, definition *ast.Document
	indentation           int
}

func (p *printingVisitor) must(_ int, err error) {
	if err != nil {
		panic(err)
	}
}

func (p *printingVisitor) printIndentation() {
	for i := 0; i < p.indentation; i++ {
		p.must(fmt.Fprintf(p.out, " "))
	}
}

func (p *printingVisitor) enter() {
	p.printIndentation()
	p.indentation += 2
}
func (p *printingVisitor) leave() {
	p.indentation -= 2
	p.printIndentation()
}

func (p *printingVisitor) EnterSchemaDefinition(ref int) {
	p.enter()
	p.must(fmt.Fprintf(p.out, "EnterSchemaDefinition: ref: %d\n", ref))
}

func (p *printingVisitor) LeaveSchemaDefinition(ref int) {
	p.leave()
	p.must(fmt.Fprintf(p.out, "LeaveSchemaDefinition: ref: %d\n\n", ref))
}

func (p *printingVisitor) EnterSchemaExtension(ref int) {
	p.enter()
	p.must(fmt.Fprintf(p.out, "EnterSchemaExtension: ref: %d\n", ref))
}

func (p *printingVisitor) LeaveSchemaExtension(ref int) {
	p.leave()
	p.must(fmt.Fprintf(p.out, "LeaveSchemaExtension: ref: %d\n\n", ref))
}

func (p *printingVisitor) EnterRootOperationTypeDefinition(ref int) {
	p.enter()
	name := p.operation.RootOperationTypeDefinitionNameString(ref)
	p.must(fmt.Fprintf(p.out, "EnterRootOperationTypeDefinition(%s): ref: %d\n", name, ref))
}

func (p *printingVisitor) LeaveRootOperationTypeDefinition(ref int) {
	p.leave()
	name := p.operation.RootOperationTypeDefinitionNameString(ref)
	p.must(fmt.Fprintf(p.out, "LeaveRootOperationTypeDefinition(%s): ref: %d\n", name, ref))
}

func (p *printingVisitor) EnterDirectiveDefinition(ref int) {
	p.enter()
	name := p.operation.DirectiveDefinitionNameString(ref)
	p.must(fmt.Fprintf(p.out, "EnterDirectiveDefinition(%s): ref: %d\n", name, ref))
}

func (p *printingVisitor) LeaveDirectiveDefinition(ref int) {
	p.leave()
	name := p.operation.DirectiveDefinitionNameString(ref)
	p.must(fmt.Fprintf(p.out, "LeaveDirectiveDefinition(%s): ref: %d\n\n", name, ref))
}

func (p *printingVisitor) EnterDirectiveLocation(location ast.DirectiveLocation) {
	p.enter()
	p.must(fmt.Fprintf(p.out, "EnterDirectiveLocation(%s)\n", location))
}

func (p *printingVisitor) LeaveDirectiveLocation(location ast.DirectiveLocation) {
	p.leave()
	p.must(fmt.Fprintf(p.out, "LeaveDirectiveLocation(%s)\n", location))
}

func (p *printingVisitor) EnterObjectTypeDefinition(ref int) {
	p.enter()
	name := p.operation.ObjectTypeDefinitionNameString(ref)
	p.must(fmt.Fprintf(p.out, "EnterObjectTypeDefinition(%s): ref: %d\n", name, ref))
}

func (p *printingVisitor) LeaveObjectTypeDefinition(ref int) {
	p.leave()
	name := p.operation.ObjectTypeDefinitionNameString(ref)
	p.must(fmt.Fprintf(p.out, "LeaveObjectTypeDefinition(%s): ref: %d\n\n", name, ref))
}

func (p *printingVisitor) EnterObjectTypeExtension(ref int) {
	p.enter()
	name := p.operation.ObjectTypeExtensionNameString(ref)
	p.must(fmt.Fprintf(p.out, "EnterObjectTypeExtension(%s): ref: %d\n", name, ref))
}

func (p *printingVisitor) LeaveObjectTypeExtension(ref int) {
	p.leave()
	name := p.operation.ObjectTypeExtensionNameString(ref)
	p.must(fmt.Fprintf(p.out, "LeaveObjectTypeExtension(%s): ref: %d\n\n", name, ref))
}

func (p *printingVisitor) EnterFieldDefinition(ref int) {
	p.enter()
	name := p.operation.FieldDefinitionNameString(ref)
	p.must(fmt.Fprintf(p.out, "EnterFieldDefinition(%s): ref: %d\n", name, ref))
}

func (p *printingVisitor) LeaveFieldDefinition(ref int) {
	p.leave()
	name := p.operation.FieldDefinitionNameString(ref)
	p.must(fmt.Fprintf(p.out, "LeaveFieldDefinition(%s): ref: %d\n", name, ref))
}

func (p *printingVisitor) EnterInputValueDefinition(ref int) {
	p.enter()
	name := p.operation.InputValueDefinitionNameString(ref)
	p.must(fmt.Fprintf(p.out, "EnterInputValueDefinition(%s): ref: %d\n", name, ref))
}

func (p *printingVisitor) LeaveInputValueDefinition(ref int) {
	p.leave()
	name := p.operation.InputValueDefinitionNameString(ref)
	p.must(fmt.Fprintf(p.out, "LeaveInputValueDefinition(%s): ref: %d\n", name, ref))
}

func (p *printingVisitor) EnterInterfaceTypeDefinition(ref int) {
	p.enter()
	name := p.operation.InterfaceTypeDefinitionNameString(ref)
	p.must(fmt.Fprintf(p.out, "EnterInterfaceTypeDefinition(%s): ref: %d\n", name, ref))
}

func (p *printingVisitor) LeaveInterfaceTypeDefinition(ref int) {
	p.leave()
	name := p.operation.InterfaceTypeDefinitionNameString(ref)
	p.must(fmt.Fprintf(p.out, "LeaveInterfaceTypeDefinition(%s): ref: %d\n\n", name, ref))
}

func (p *printingVisitor) EnterInterfaceTypeExtension(ref int) {
	p.enter()
	name := p.operation.InterfaceTypeExtensionNameString(ref)
	p.must(fmt.Fprintf(p.out, "EnterInterfaceTypeExtension(%s): ref: %d\n", name, ref))
}

func (p *printingVisitor) LeaveInterfaceTypeExtension(ref int) {
	p.leave()
	name := p.operation.InterfaceTypeExtensionNameString(ref)
	p.must(fmt.Fprintf(p.out, "LeaveInterfaceTypeExtension(%s): ref: %d\n\n", name, ref))
}

func (p *printingVisitor) EnterScalarTypeDefinition(ref int) {
	p.enter()
	name := p.operation.ScalarTypeDefinitionNameString(ref)
	p.must(fmt.Fprintf(p.out, "EnterScalarTypeDefinition(%s): ref: %d\n", name, ref))
}

func (p *printingVisitor) LeaveScalarTypeDefinition(ref int) {
	p.leave()
	name := p.operation.ScalarTypeDefinitionNameString(ref)
	p.must(fmt.Fprintf(p.out, "LeaveScalarTypeDefinition(%s): ref: %d\n\n", name, ref))
}

func (p *printingVisitor) EnterScalarTypeExtension(ref int) {
	p.enter()
	name := p.operation.ScalarTypeExtensionNameString(ref)
	p.must(fmt.Fprintf(p.out, "EnterScalarTypeExtension(%s): ref: %d\n", name, ref))
}

func (p *printingVisitor) LeaveScalarTypeExtension(ref int) {
	p.leave()
	name := p.operation.ScalarTypeExtensionNameString(ref)
	p.must(fmt.Fprintf(p.out, "LeaveScalarTypeExtension(%s): ref: %d\n\n", name, ref))
}

func (p *printingVisitor) EnterUnionTypeDefinition(ref int) {
	p.enter()
	name := p.operation.UnionTypeDefinitionNameString(ref)
	p.must(fmt.Fprintf(p.out, "EnterUnionTypeDefinition(%s): ref: %d\n", name, ref))
}

func (p *printingVisitor) LeaveUnionTypeDefinition(ref int) {
	p.leave()
	name := p.operation.UnionTypeDefinitionNameString(ref)
	p.must(fmt.Fprintf(p.out, "LeaveUnionTypeDefinition(%s): ref: %d\n\n", name, ref))
}

func (p *printingVisitor) EnterUnionTypeExtension(ref int) {
	p.enter()
	name := p.operation.UnionTypeExtensionNameString(ref)
	p.must(fmt.Fprintf(p.out, "EnterUnionTypeExtension(%s): ref: %d\n", name, ref))
}

func (p *printingVisitor) LeaveUnionTypeExtension(ref int) {
	p.leave()
	name := p.operation.UnionTypeExtensionNameString(ref)
	p.must(fmt.Fprintf(p.out, "LeaveUnionTypeExtension(%s): ref: %d\n\n", name, ref))
}

func (p *printingVisitor) EnterUnionMemberType(ref int) {
	p.enter()
	name := p.operation.TypeNameString(ref)
	p.must(fmt.Fprintf(p.out, "EnterUnionMemberType(%s): ref: %d\n", name, ref))
}

func (p *printingVisitor) LeaveUnionMemberType(ref int) {
	p.leave()
	name := p.operation.TypeNameString(ref)
	p.must(fmt.Fprintf(p.out, "LeaveUnionMemberType(%s): ref: %d\n", name, ref))
}

func (p *printingVisitor) EnterEnumTypeDefinition(ref int) {
	p.enter()
	name := p.operation.EnumTypeDefinitionNameString(ref)
	p.must(fmt.Fprintf(p.out, "EnterEnumTypeDefinition(%s): ref: %d\n", name, ref))
}

func (p *printingVisitor) LeaveEnumTypeDefinition(ref int) {
	p.leave()
	name := p.operation.EnumTypeDefinitionNameString(ref)
	p.must(fmt.Fprintf(p.out, "LeaveEnumTypeDefinition(%s): ref: %d\n\n", name, ref))
}

func (p *printingVisitor) EnterEnumTypeExtension(ref int) {
	p.enter()
	name := p.operation.EnumTypeExtensionNameString(ref)
	p.must(fmt.Fprintf(p.out, "EnterEnumTypeExtension(%s): ref: %d\n", name, ref))
}

func (p *printingVisitor) LeaveEnumTypeExtension(ref int) {
	p.leave()
	name := p.operation.EnumTypeExtensionNameString(ref)
	p.must(fmt.Fprintf(p.out, "LeaveEnumTypeExtension(%s): ref: %d\n\n", name, ref))
}

func (p *printingVisitor) EnterEnumValueDefinition(ref int) {
	p.enter()
	name := p.operation.EnumValueDefinitionNameString(ref)
	p.must(fmt.Fprintf(p.out, "EnterEnumValueDefinition(%s): ref: %d\n", name, ref))
}

func (p *printingVisitor) LeaveEnumValueDefinition(ref int) {
	p.leave()
	name := p.operation.EnumValueDefinitionNameString(ref)
	p.must(fmt.Fprintf(p.out, "LeaveEnumValueDefinition(%s): ref: %d\n", name, ref))
}

func (p *printingVisitor) EnterInputObjectTypeDefinition(ref int) {
	p.enter()
	name := p.operation.InputObjectTypeDefinitionNameString(ref)
	p.must(fmt.Fprintf(p.out, "EnterInputObjectTypeDefinition(%s): ref: %d\n", name, ref))
}

func (p *printingVisitor) LeaveInputObjectTypeDefinition(ref int) {
	p.leave()
	name := p.operation.InputObjectTypeDefinitionNameString(ref)
	p.must(fmt.Fprintf(p.out, "LeaveInputObjectTypeDefinition(%s): ref: %d\n\n", name, ref))
}

func (p *printingVisitor) EnterInputObjectTypeExtension(ref int) {
	p.enter()
	name := p.operation.InputObjectTypeExtensionNameString(ref)
	p.must(fmt.Fprintf(p.out, "EnterInputObjectTypeExtension(%s): ref: %d\n", name, ref))
}

func (p *printingVisitor) LeaveInputObjectTypeExtension(ref int) {
	p.leave()
	name := p.operation.InputObjectTypeExtensionNameString(ref)
	p.must(fmt.Fprintf(p.out, "LeaveInputObjectTypeExtension(%s): ref: %d\n\n", name, ref))
}

func (p *printingVisitor) EnterDocument(operation, definition *ast.Document) {
	p.operation, p.definition = operation, definition
	p.must(fmt.Fprintf(p.out, "EnterDocument\n"))
}

func (p *printingVisitor) LeaveDocument(operation, definition *ast.Document) {
	p.must(fmt.Fprintf(p.out, "LeaveDocument\n"))
}

func (p *printingVisitor) EnterDirective(ref int) {
	p.enter()
	name := p.operation.DirectiveNameString(ref)
	p.must(fmt.Fprintf(p.out, "EnterDirective(%s): ref: %d\n", name, ref))
}

func (p *printingVisitor) LeaveDirective(ref int) {
	p.leave()
	name := p.operation.DirectiveNameString(ref)
	p.must(fmt.Fprintf(p.out, "LeaveDirective(%s): ref: %d\n", name, ref))
}

func (p *printingVisitor) EnterVariableDefinition(ref int) {
	p.enter()
	varName := string(p.operation.VariableValueNameBytes(p.operation.VariableDefinitions[ref].VariableValue.Ref))
	p.must(fmt.Fprintf(p.out, "EnterVariableDefinition(%s): ref: %d\n", varName, ref))
}

func (p *printingVisitor) LeaveVariableDefinition(ref int) {
	p.leave()
	varName := string(p.operation.VariableValueNameBytes(p.operation.VariableDefinitions[ref].VariableValue.Ref))
	p.must(fmt.Fprintf(p.out, "LeaveVariableDefinition(%s): ref: %d\n", varName, ref))
}

func (p *printingVisitor) EnterOperationDefinition(ref int) {
	p.enter()
	name := p.operation.Input.ByteSliceString(p.operation.OperationDefinitions[ref].Name)
	if name == "" {
		name = "anonymous!"
	}
	p.must(fmt.Fprintf(p.out, "EnterOperationDefinition (%s): ref: %d\n", name, ref))

}

func (p *printingVisitor) LeaveOperationDefinition(ref int) {
	p.leave()
	name := p.operation.Input.ByteSliceString(p.operation.OperationDefinitions[ref].Name)
	if name == "" {
		name = "anonymous!"
	}
	p.must(fmt.Fprintf(p.out, "LeaveOperationDefinition(%s): ref: %d\n\n", name, ref))
}

func (p *printingVisitor) EnterSelectionSet(ref int) {
	p.enter()
	parentTypeName := p.definition.NodeNameString(p.EnclosingTypeDefinition)
	p.must(fmt.Fprintf(p.out, "EnterSelectionSet(%s): ref: %d\n", parentTypeName, ref))
}

func (p *printingVisitor) LeaveSelectionSet(ref int) {
	p.leave()
	parentTypeName := p.definition.NodeNameString(p.EnclosingTypeDefinition)
	p.must(fmt.Fprintf(p.out, "LeaveSelectionSet(%s): ref: %d\n", parentTypeName, ref))
}

func (p *printingVisitor) EnterField(ref int) {
	p.enter()
	fieldName := p.operation.FieldNameBytes(ref)
	parentTypeName := p.definition.NodeNameString(p.EnclosingTypeDefinition)
	p.must(fmt.Fprintf(p.out, "EnterField(%s::%s): ref: %d\n", fieldName, parentTypeName, ref))
}

func (p *printingVisitor) LeaveField(ref int) {
	p.leave()
	fieldName := p.operation.FieldNameString(ref)
	parentTypeName := p.definition.NodeNameString(p.EnclosingTypeDefinition)
	p.must(fmt.Fprintf(p.out, "LeaveField(%s::%s): ref: %d\n", fieldName, parentTypeName, ref))
}

func (p *printingVisitor) EnterArgument(ref int) {
	p.enter()
	argName := p.operation.ArgumentNameString(ref)
	parentTypeName := p.definition.NodeNameString(p.EnclosingTypeDefinition)
	p.must(fmt.Fprintf(p.out, "EnterArgument(%s::%s): ref: %d\n", argName, parentTypeName, ref))
}

func (p *printingVisitor) LeaveArgument(ref int) {
	p.leave()
	argName := p.operation.ArgumentNameString(ref)
	parentTypeName := p.definition.NodeNameString(p.EnclosingTypeDefinition)
	p.must(fmt.Fprintf(p.out, "LeaveArgument(%s::%s): ref: %d\n", argName, parentTypeName, ref))
}

func (p *printingVisitor) EnterFragmentSpread(ref int) {
	p.enter()
	spreadName := p.operation.FragmentSpreadNameString(ref)
	p.must(fmt.Fprintf(p.out, "EnterFragmentSpread(%s): ref: %d\n", spreadName, ref))
}

func (p *printingVisitor) LeaveFragmentSpread(ref int) {
	p.leave()
	spreadName := p.operation.FragmentSpreadNameString(ref)
	p.must(fmt.Fprintf(p.out, "LeaveFragmentSpread(%s): ref: %d\n", spreadName, ref))
}

func (p *printingVisitor) EnterInlineFragment(ref int) {
	p.enter()
	typeConditionName := p.operation.InlineFragmentTypeConditionNameString(ref)
	if typeConditionName == "" {
		typeConditionName = "anonymous!"
	}
	p.must(fmt.Fprintf(p.out, "EnterInlineFragment(%s): ref: %d\n", typeConditionName, ref))
}

func (p *printingVisitor) LeaveInlineFragment(ref int) {
	p.leave()
	typeConditionName := p.operation.InlineFragmentTypeConditionNameString(ref)
	if typeConditionName == "" {
		typeConditionName = "anonymous!"
	}
	p.must(fmt.Fprintf(p.out, "LeaveInlineFragment(%s): ref: %d\n", typeConditionName, ref))
}

func (p *printingVisitor) EnterFragmentDefinition(ref int) {
	p.enter()
	name := p.operation.FragmentDefinitionNameString(ref)
	p.must(fmt.Fprintf(p.out, "EnterFragmentDefinition(%s): ref: %d\n", name, ref))
}

func (p *printingVisitor) LeaveFragmentDefinition(ref int) {
	p.leave()
	name := p.operation.FragmentDefinitionNameString(ref)
	p.must(fmt.Fprintf(p.out, "LeaveFragmentDefinition(%s): ref: %d\n\n", name, ref))
}
