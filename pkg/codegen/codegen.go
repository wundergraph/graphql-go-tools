package codegen

import (
	"fmt"
	. "github.com/dave/jennifer/jen"
	"github.com/iancoleman/strcase"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
	"io"
)

type CodeGen struct {
	doc         *ast.Document
	packageName string
	file        *File
	walker      *astvisitor.Walker
	report      *operationreport.Report
}

func NewCodeGen(doc *ast.Document, packageName string) *CodeGen {
	return &CodeGen{
		doc:         doc,
		packageName: packageName,
	}
}

func (c *CodeGen) Generate(w io.Writer) (int, error) {
	c.file = NewFile(c.packageName)

	c.report = &operationreport.Report{}
	walker := astvisitor.NewWalker(48)
	c.walker = &walker

	c.walker.RegisterAllNodesVisitor(&genVisitor{
		doc:    c.doc,
		Walker: c.walker,
		file:   c.file,
	})

	c.report.Reset()
	c.walker.Walk(c.doc, c.doc, c.report)

	return fmt.Fprintf(w, "%#v", c.file)
}

type genVisitor struct {
	doc *ast.Document
	*astvisitor.Walker
	file *File
}

func (g *genVisitor) camelCase(in string) string {
	return strcase.ToCamel(in)
}

func (g *genVisitor) renderType(stmt *Statement, ref int, nullable bool) {
	graphqlType := g.doc.Types[ref]
	switch graphqlType.TypeKind {
	case ast.TypeKindNamed:
		if nullable {
			stmt.Id("*")
		}
		typeName := g.doc.TypeNameString(ref)
		switch typeName {
		case "Boolean":
			stmt.Bool()
		case "String":
			stmt.String()
		case "Int":
			stmt.Int64()
		case "Float":
			stmt.Int32()
		default:
			stmt.Id(typeName)
		}
	case ast.TypeKindNonNull:
		g.renderType(stmt, graphqlType.OfType, false)
	case ast.TypeKindList:
		if nullable {
			stmt.Id("*")
		}
		g.renderType(stmt.Id("[]"), graphqlType.OfType, true)
	}
}

func (g *genVisitor) EnterDocument(operation, definition *ast.Document) {

}

func (g *genVisitor) LeaveDocument(operation, definition *ast.Document) {

}

func (g *genVisitor) EnterObjectTypeDefinition(ref int) {

}

func (g *genVisitor) LeaveObjectTypeDefinition(ref int) {

}

func (g *genVisitor) EnterObjectTypeExtension(ref int) {

}

func (g *genVisitor) LeaveObjectTypeExtension(ref int) {

}

func (g *genVisitor) EnterFieldDefinition(ref int) {

}

func (g *genVisitor) LeaveFieldDefinition(ref int) {

}

func (g *genVisitor) EnterInputValueDefinition(ref int) {

}

func (g *genVisitor) LeaveInputValueDefinition(ref int) {

}

func (g *genVisitor) EnterInterfaceTypeDefinition(ref int) {

}

func (g *genVisitor) LeaveInterfaceTypeDefinition(ref int) {

}

func (g *genVisitor) EnterInterfaceTypeExtension(ref int) {

}

func (g *genVisitor) LeaveInterfaceTypeExtension(ref int) {

}

func (g *genVisitor) EnterScalarTypeDefinition(ref int) {

}

func (g *genVisitor) LeaveScalarTypeDefinition(ref int) {

}

func (g *genVisitor) EnterScalarTypeExtension(ref int) {

}

func (g *genVisitor) LeaveScalarTypeExtension(ref int) {

}

func (g *genVisitor) EnterUnionTypeDefinition(ref int) {

}

func (g *genVisitor) LeaveUnionTypeDefinition(ref int) {

}

func (g *genVisitor) EnterUnionTypeExtension(ref int) {

}

func (g *genVisitor) LeaveUnionTypeExtension(ref int) {

}

func (g *genVisitor) EnterUnionMemberType(ref int) {

}

func (g *genVisitor) LeaveUnionMemberType(ref int) {

}

func (g *genVisitor) EnterEnumTypeDefinition(ref int) {

}

func (g *genVisitor) LeaveEnumTypeDefinition(ref int) {
	name := g.doc.EnumTypeDefinitionNameString(ref)
	g.file.Type().Id(name).Int()
	refs := g.doc.EnumTypeDefinitions[ref].EnumValuesDefinition.Refs
	if len(refs) == 0 {
		return
	}
	g.file.Const().DefsFunc(func(group *Group) {
		group.Id("UNDEFINED_" + name).Id(name).Op("=").Id("iota")
		for _, i := range refs {
			valueName := g.doc.EnumValueDefinitionNameString(i)
			group.Id(name + "_" + valueName)
		}
	})
}

func (g *genVisitor) EnterEnumTypeExtension(ref int) {

}

func (g *genVisitor) LeaveEnumTypeExtension(ref int) {

}

func (g *genVisitor) EnterEnumValueDefinition(ref int) {

}

func (g *genVisitor) LeaveEnumValueDefinition(ref int) {

}

func (g *genVisitor) EnterInputObjectTypeDefinition(ref int) {
	structName := g.doc.InputObjectTypeDefinitionNameString(ref)
	g.file.Type().Id(structName).StructFunc(func(group *Group) {
		for _, i := range g.doc.InputObjectTypeDefinitions[ref].InputFieldsDefinition.Refs {
			name := g.camelCase(g.doc.InputValueDefinitionNameString(i))
			stmt := group.Id(name)
			g.renderType(stmt, g.doc.InputValueDefinitionType(i), true)
		}
	})

}

func (g *genVisitor) LeaveInputObjectTypeDefinition(ref int) {

}

func (g *genVisitor) EnterInputObjectTypeExtension(ref int) {

}

func (g *genVisitor) LeaveInputObjectTypeExtension(ref int) {

}

func (g *genVisitor) EnterDirectiveDefinition(ref int) {
	structName := g.camelCase(g.doc.DirectiveDefinitionNameString(ref))
	g.file.Type().Id(structName).StructFunc(func(group *Group) {
		for _, i := range g.doc.DirectiveDefinitions[ref].ArgumentsDefinition.Refs {
			name := g.camelCase(g.doc.InputValueDefinitionNameString(i))
			stmt := group.Id(name)
			g.renderType(stmt, g.doc.InputValueDefinitionType(i), true)
		}
	})
}

func (g *genVisitor) LeaveDirectiveDefinition(ref int) {

}

func (g *genVisitor) EnterDirectiveLocation(location ast.DirectiveLocation) {

}

func (g *genVisitor) LeaveDirectiveLocation(location ast.DirectiveLocation) {

}

func (g *genVisitor) EnterSchemaDefinition(ref int) {

}

func (g *genVisitor) LeaveSchemaDefinition(ref int) {

}

func (g *genVisitor) EnterSchemaExtension(ref int) {

}

func (g *genVisitor) LeaveSchemaExtension(ref int) {

}

func (g *genVisitor) EnterRootOperationTypeDefinition(ref int) {

}

func (g *genVisitor) LeaveRootOperationTypeDefinition(ref int) {

}

func (g *genVisitor) EnterOperationDefinition(ref int) {

}

func (g *genVisitor) LeaveOperationDefinition(ref int) {

}

func (g *genVisitor) EnterSelectionSet(ref int) {

}

func (g *genVisitor) LeaveSelectionSet(ref int) {

}

func (g *genVisitor) EnterField(ref int) {

}

func (g *genVisitor) LeaveField(ref int) {

}

func (g *genVisitor) EnterArgument(ref int) {

}

func (g *genVisitor) LeaveArgument(ref int) {

}

func (g *genVisitor) EnterFragmentSpread(ref int) {

}

func (g *genVisitor) LeaveFragmentSpread(ref int) {

}

func (g *genVisitor) EnterInlineFragment(ref int) {

}

func (g *genVisitor) LeaveInlineFragment(ref int) {

}

func (g *genVisitor) EnterFragmentDefinition(ref int) {

}

func (g *genVisitor) LeaveFragmentDefinition(ref int) {

}

func (g *genVisitor) EnterVariableDefinition(ref int) {

}

func (g *genVisitor) LeaveVariableDefinition(ref int) {

}

func (g *genVisitor) EnterDirective(ref int) {

}

func (g *genVisitor) LeaveDirective(ref int) {

}