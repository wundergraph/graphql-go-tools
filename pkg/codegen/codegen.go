// Package codegen generates code to make using this library easier
// You can currently use the code generator to generate go structs and Unmarshal methods for Directives and Input Objects type definitions
// This helps you interact very easily with configuration supplied by Directives which you can easily unmarshal into go structs
package codegen

import (
	"fmt"
	 "github.com/dave/jennifer/jen"
	"github.com/iancoleman/strcase"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
	"io"
	"strings"
)

type CodeGen struct {
	doc    *ast.Document
	file   *jen.File
	walker *astvisitor.Walker
	report *operationreport.Report
	config Config
}

type Config struct {
	PackageName           string
	DirectiveStructSuffix string
}

func New(doc *ast.Document, config Config) *CodeGen {
	return &CodeGen{
		doc:    doc,
		config: config,
	}
}

func (c *CodeGen) Generate(w io.Writer) (int, error) {
	c.file = jen.NewFile(c.config.PackageName)
	c.file.PackageComment("Code generated by graphql-go-tools gen, DO NOT EDIT.")
	c.report = &operationreport.Report{}
	walker := astvisitor.NewWalker(48)
	c.walker = &walker

	c.walker.RegisterAllNodesVisitor(&genVisitor{
		doc:    c.doc,
		Walker: c.walker,
		file:   c.file,
		config: c.config,
	})

	c.report.Reset()
	c.walker.Walk(c.doc, c.doc, c.report)

	return fmt.Fprintf(w, "%#v", c.file)
}

type genVisitor struct {
	doc *ast.Document
	*astvisitor.Walker
	file   *jen.File
	config Config
}

func (g *genVisitor) renderType(stmt *jen.Statement, ref int, nullable bool) {
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
			stmt.Float32()
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

func (g *genVisitor) renderUnmarshal(structName string, ref ast.Node) {

	switch ref.Kind {
	case ast.NodeKindDirectiveDefinition:
		g.file.Func().Params(jen.Id(strings.ToLower(structName)[0:1]).Id("*").Id(structName)).
			Id("Unmarshal").Params(
			jen.Id("doc").Id("*").Qual("github.com/jensneuse/graphql-go-tools/pkg/ast", "Document"),
			jen.Id("ref").Int()).
			BlockFunc(func(group *jen.Group) {
				group.For(
					jen.Id("_").Op(",").Id("i").Op(":=").Range().Id("doc").Dot("Directives").Index(jen.Id("ref")).Dot("Arguments").Dot("Refs"),
				).BlockFunc(func(group *jen.Group) {
					group.Id("name").Op(":=").Id("doc").Dot("ArgumentNameString").Call(jen.Id("i"))
					group.Switch(jen.Id("name")).BlockFunc(func(group *jen.Group) {
						for _, i := range g.doc.DirectiveDefinitions[ref.Ref].ArgumentsDefinition.Refs {
							g.renderInputValueDefinitionSwitchCase(group, structName, i, "ArgumentValue")
						}
					})
				})
			})
	case ast.NodeKindInputObjectTypeDefinition:
		g.file.Func().Params(jen.Id(strings.ToLower(structName)[0:1]).Id("*").Id(structName)).
			Id("Unmarshal").Params(
			jen.Id("doc").Id("*").Qual("github.com/jensneuse/graphql-go-tools/pkg/ast", "Document"),
			jen.Id("ref").Int()).
			BlockFunc(func(group *jen.Group) {
				group.For(
					jen.Id("_").Op(",").Id("i").Op(":=").Range().Id("doc").Dot("ObjectValues").Index(jen.Id("ref")).Dot("Refs"),
				).BlockFunc(func(group *jen.Group) {
					group.Id("name").Op(":=").String().Call(jen.Id("doc").Dot("ObjectFieldNameBytes").Call(jen.Id("i")))
					group.Switch(jen.Id("name")).BlockFunc(func(group *jen.Group) {
						for _, i := range g.doc.InputObjectTypeDefinitions[ref.Ref].InputFieldsDefinition.Refs {
							g.renderInputValueDefinitionSwitchCase(group, structName, i, "ObjectFieldValue")
						}
					})
				})
			})
	}
}

func (g *genVisitor) renderInputValueDefinitionSwitchCase(group *jen.Group, structName string, ref int, valueSourceName string) {
	valueName := g.doc.InputValueDefinitionNameString(ref)
	fieldName := strcase.ToCamel(g.doc.InputValueDefinitionNameString(ref))
	valueTypeRef := g.doc.InputValueDefinitionType(ref)
	g.renderInputValueDefinitionType(group, valueName, fieldName, structName, valueTypeRef, valueSourceName, ast.TypeKindUnknown)
}

func (g *genVisitor) renderInputValueDefinitionType(group *jen.Group, valueName, fieldName, structName string, ref int, valueSourceName string, parentTypeKind ast.TypeKind) {
	typeKind := g.doc.Types[ref].TypeKind
	switch typeKind {
	case ast.TypeKindNamed:
		typeName := g.doc.TypeNameString(ref)
		valueAssignment := g.valueAssingmentStatement(typeName, valueSourceName, false)
		if valueAssignment == nil {
			return
		}
		switch parentTypeKind {
		case ast.TypeKindNonNull:
			group.Case(jen.Lit(valueName)).Block(
				valueAssignment,
				jen.Id(strings.ToLower(structName[0:1])).Dot(fieldName).Op("=").Id("val"),
			)
		default:
			group.Case(jen.Lit(valueName)).Block(
				valueAssignment,
				jen.Id(strings.ToLower(structName[0:1])).Dot(fieldName).Op("=").Id("&").Id("val"),
			)
		}
	case ast.TypeKindNonNull:
		g.renderInputValueDefinitionType(group, valueName, fieldName, structName, g.doc.Types[ref].OfType, valueSourceName, typeKind)
	case ast.TypeKindList:
		listType := &jen.Statement{}
		g.renderType(listType, ref, false)
		listDefinition := jen.Id("list").Op(":=").Make(listType, jen.Id("0"), jen.Len(jen.Id("doc").Dot("ListValues").Index(jen.Id("doc").Dot(valueSourceName).Call(jen.Id("i")).Dot("Ref")).Dot("Refs")))
		switch parentTypeKind {
		case ast.TypeKindNonNull:
			switch g.doc.Types[g.doc.Types[ref].OfType].TypeKind {
			case ast.TypeKindNonNull:
				// non null list of non null type
				typeName := g.doc.TypeNameString(g.doc.Types[g.doc.Types[ref].OfType].OfType)
				group.Case(jen.Lit(valueName)).Block(
					listDefinition,
					jen.For(jen.Id("_").Op(",").Id("i").Op(":=").Range().Id("doc").Dot("ListValues").Index(jen.Id("doc").Dot(valueSourceName).Call(jen.Id("i")).Dot("Ref")).Dot("Refs")).Block(
						g.valueAssingmentStatement(typeName, valueSourceName, true),
						jen.Id("list").Op("=").Append(jen.Id("list").Op(",").Id("val")),
					),
					jen.Id(strings.ToLower(structName[0:1])).Dot(fieldName).Op("=").Id("list"),
				)
			case ast.TypeKindNamed:
				// non null list of nullable type
				typeName := g.doc.TypeNameString(g.doc.Types[ref].OfType)
				group.Case(jen.Lit(valueName)).Block(
					listDefinition,
					jen.For(jen.Id("_").Op(",").Id("i").Op(":=").Range().Id("doc").Dot("ListValues").Index(jen.Id("doc").Dot(valueSourceName).Call(jen.Id("i")).Dot("Ref")).Dot("Refs")).Block(
						g.valueAssingmentStatement(typeName, valueSourceName, true),
						jen.Id("list").Op("=").Append(jen.Id("list").Op(",").Id("&").Id("val")),
					),
					jen.Id(strings.ToLower(structName[0:1])).Dot(fieldName).Op("=").Id("list"),
				)
			}
		default:
			switch g.doc.Types[g.doc.Types[ref].OfType].TypeKind {
			case ast.TypeKindNonNull:
				// nullable list of non null type
				typeName := g.doc.TypeNameString(g.doc.Types[g.doc.Types[ref].OfType].OfType)
				group.Case(jen.Lit(valueName)).Block(
					listDefinition,
					jen.For(jen.Id("_").Op(",").Id("i").Op(":=").Range().Id("doc").Dot("ListValues").Index(jen.Id("doc").Dot(valueSourceName).Call(jen.Id("i")).Dot("Ref")).Dot("Refs")).Block(
						g.valueAssingmentStatement(typeName, valueSourceName, true),
						jen.Id("list").Op("=").Append(jen.Id("list").Op(",").Id("val")),
					),
					jen.Id(strings.ToLower(structName[0:1])).Dot(fieldName).Op("=").Id("&").Id("list"),
				)
			default:
				// nullable list of nullable type
				typeName := g.doc.TypeNameString(g.doc.Types[ref].OfType)
				group.Case(jen.Lit(valueName)).Block(
					listDefinition,
					jen.For(jen.Id("_").Op(",").Id("i").Op(":=").Range().Id("doc").Dot("ListValues").Index(jen.Id("doc").Dot(valueSourceName).Call(jen.Id("i")).Dot("Ref")).Dot("Refs")).Block(
						g.valueAssingmentStatement(typeName, valueSourceName, true),
						jen.Id("list").Op("=").Append(jen.Id("list").Op(",").Id("&").Id("val")),
					),
					jen.Id(strings.ToLower(structName[0:1])).Dot(fieldName).Op("=").Id("&").Id("list"),
				)
			}
		}
	}
}

func (g *genVisitor) valueAssingmentStatement(scalarName, valueSourceName string, insideList bool) *jen.Statement {

	var caller *jen.Statement
	if insideList {
		caller = jen.Id("doc").Dot("Value").Call(jen.Id("i")).Dot("Ref")
	} else {
		caller = jen.Id("doc").Dot(valueSourceName).Call(jen.Id("i")).Dot("Ref")
	}

	switch scalarName {
	case "String":
		return jen.Id("val").Op(":=").Id("doc").Dot("StringValueContentString").Call(caller)
	case "Boolean":
		return jen.Id("val").Op(":=").Id("bool").Call(jen.Id("doc").Dot("BooleanValue").Call(caller))
	case "Float":
		return jen.Id("val").Op(":=").Id("doc").Dot("FloatValueAsFloat32").Call(caller)
	case "Int":
		return jen.Id("val").Op(":=").Id("doc").Dot("IntValueAsInt").Call(caller)
	default:
		def := jen.Var().Id("val").Id(scalarName).Line().Id("val").Dot("Unmarshal").Call(jen.Id("doc"), caller)
		return def
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
	shortHandle := strings.ToLower(name)[0:1]
	g.file.Type().Id(name).Int()
	refs := g.doc.EnumTypeDefinitions[ref].EnumValuesDefinition.Refs

	g.file.Func().Params(jen.Id(shortHandle).Id("*").Id(name)).Id("Unmarshal").Params(jen.Id("doc").Id("*").Qual("github.com/jensneuse/graphql-go-tools/pkg/ast", "Document"), jen.Id("ref").Int()).Block(
		jen.Switch(jen.Id("doc").Dot("EnumValueNameString").Call(jen.Id("ref"))).BlockFunc(func(group *jen.Group) {
			for _, i := range refs {
				valueName := g.doc.EnumValueDefinitionNameString(i)
				group.Case(jen.Lit(valueName)).Block(
					jen.Id("*").Id(shortHandle).Op("=").Id(name + "_" + valueName),
				)
			}
		}),
	)

	if len(refs) == 0 {
		return
	}
	g.file.Const().DefsFunc(func(group *jen.Group) {
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
	g.file.Type().Id(structName).StructFunc(func(group *jen.Group) {
		for _, i := range g.doc.InputObjectTypeDefinitions[ref].InputFieldsDefinition.Refs {
			name := strcase.ToCamel(g.doc.InputValueDefinitionNameString(i))
			stmt := group.Id(name)
			g.renderType(stmt, g.doc.InputValueDefinitionType(i), true)
		}
	})
	g.renderUnmarshal(structName, ast.Node{Kind: ast.NodeKindInputObjectTypeDefinition, Ref: ref})
}

func (g *genVisitor) LeaveInputObjectTypeDefinition(ref int) {

}

func (g *genVisitor) EnterInputObjectTypeExtension(ref int) {

}

func (g *genVisitor) LeaveInputObjectTypeExtension(ref int) {

}

func (g *genVisitor) EnterDirectiveDefinition(ref int) {
	structName := strcase.ToCamel(g.doc.DirectiveDefinitionNameString(ref)) + g.config.DirectiveStructSuffix
	g.file.Type().Id(structName).StructFunc(func(group *jen.Group) {
		for _, i := range g.doc.DirectiveDefinitions[ref].ArgumentsDefinition.Refs {
			name := strcase.ToCamel(g.doc.InputValueDefinitionNameString(i))
			stmt := group.Id(name)
			g.renderType(stmt, g.doc.InputValueDefinitionType(i), true)
		}
	})
	g.renderUnmarshal(structName, ast.Node{Kind: ast.NodeKindDirectiveDefinition, Ref: ref})
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
