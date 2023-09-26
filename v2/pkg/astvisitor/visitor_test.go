package astvisitor_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/jensneuse/diffview"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/testing/goldie"
)

var must = func(err error) {
	if err != nil {
		panic(err)
	}
}

func TestVisitOperation(t *testing.T) {

	definition := unsafeparser.ParseGraphqlDocumentString(testDefinition)
	operation := unsafeparser.ParseGraphqlDocumentString(testOperation)
	report := operationreport.Report{}

	walker := astvisitor.NewWalker(48)
	buff := &bytes.Buffer{}
	visitor := &printingVisitor{
		Walker:     &walker,
		out:        buff,
		operation:  &operation,
		definition: &definition,
	}

	walker.RegisterAllNodesVisitor(visitor)

	walker.Walk(&operation, &definition, &report)

	out := buff.Bytes()
	goldie.Assert(t, "visitor", out)

	if t.Failed() {

		fixture, err := os.ReadFile("./fixtures/visitor.golden")
		if err != nil {
			t.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes("introspection_lexed", fixture, out)
	}
}

func TestVisitWithTypeName(t *testing.T) {
	operation := `
		query getApi($id: String!) {
		  api(id: $id) {
			__typename
			... on Api {
			  __typename
			  id
			  name
			}
			... on RequestResult {
			  __typename
			  status
			  message
			}  
		  }
		}`
	definition := `
		schema {
			query: Query
		}
		type Query {
			api(id: String): ApiResult
		}
		union ApiResult = Api | RequestResult
		type Api {
			id: String
			name: String
		}
		type RequestResult {
			status: String
			message: String
		}
		scalar String
	`

	walker := astvisitor.NewWalker(8)
	op := unsafeparser.ParseGraphqlDocumentString(operation)
	def := unsafeparser.ParseGraphqlDocumentString(definition)
	var report operationreport.Report
	walker.Walk(&op, &def, &report)
	if report.HasErrors() {
		t.Fatal(report.Error())
	}
}

func TestVisitSchemaDefinition(t *testing.T) {

	operation := unsafeparser.ParseGraphqlDocumentString(testDefinitions)
	report := operationreport.Report{}
	walker := astvisitor.NewWalker(48)
	buff := &bytes.Buffer{}
	visitor := &printingVisitor{
		Walker:    &walker,
		out:       buff,
		operation: &operation,
	}

	walker.RegisterAllNodesVisitor(visitor)

	walker.Walk(&operation, nil, &report)
	if report.HasErrors() {
		t.Fatal(report.Error())
	}

	out := buff.Bytes()
	goldie.Assert(t, "schema_visitor", out)

	if t.Failed() {

		fixture, err := os.ReadFile("./fixtures/schema_visitor.golden")
		if err != nil {
			t.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes("schema_visitor", fixture, out)
	}
}

func TestWalker_Path(t *testing.T) {

	definition := unsafeparser.ParseGraphqlDocumentString(testDefinition)
	operation := unsafeparser.ParseGraphqlDocumentString(`
		query {
			posts {
				id
				description
				user {
					id
					name
				}
			}
		}

		query MyQuery {
			posts {
				id
				description
				user {
					id
					name
					posts {
						id
					}
				}
			}
		}`)

	walker := astvisitor.NewWalker(48)
	buff := &bytes.Buffer{}
	report := operationreport.Report{}
	pathVisitor := pathVisitor{
		Walker: &walker,
		out:    buff,
	}

	walker.RegisterEnterDocumentVisitor(&pathVisitor)
	walker.RegisterEnterFieldVisitor(&pathVisitor)

	walker.Walk(&operation, &definition, &report)

	if report.HasErrors() {
		t.Fatal(report.Error())
	}

	out := buff.Bytes()
	goldie.Assert(t, "path", out)

	if t.Failed() {

		fixture, err := os.ReadFile("./fixtures/path.golden")
		if err != nil {
			t.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes("path", fixture, out)
	}
}

type pathVisitor struct {
	*astvisitor.Walker
	out     *bytes.Buffer
	op, def *ast.Document
}

func (p *pathVisitor) EnterDocument(operation, definition *ast.Document) {
	p.op, p.def = operation, definition
}

func (p *pathVisitor) EnterField(ref int) {
	p.out.Write([]byte(fmt.Sprintf("EnterField: %s, path: %s\n", p.op.FieldNameUnsafeString(ref), p.Path)))
}

func TestVisitWithSkip(t *testing.T) {

	definition := unsafeparser.ParseGraphqlDocumentString(testDefinition)
	operation := unsafeparser.ParseGraphqlDocumentString(`
		query PostsUserQuery {
			posts {
				id
				description
				user {
					id
					name
				}
			}
		}`)

	walker := astvisitor.NewWalker(48)
	buff := &bytes.Buffer{}
	visitor := &printingVisitor{
		Walker:     &walker,
		out:        buff,
		operation:  &operation,
		definition: &definition,
	}
	report := operationreport.Report{}

	skipUser := skipUserVisitor{
		Walker: &walker,
	}

	walker.RegisterEnterDocumentVisitor(&skipUser)
	walker.RegisterEnterFieldVisitor(&skipUser)
	walker.RegisterAllNodesVisitor(visitor)

	walker.Walk(&operation, &definition, &report)

	out := buff.Bytes()
	goldie.Assert(t, "visitor_skip", out)

	if t.Failed() {

		fixture, err := os.ReadFile("./fixtures/visitor_skip.golden")
		if err != nil {
			t.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes("introspection_lexed", fixture, out)
	}
}

type skipUserVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
}

func (s *skipUserVisitor) EnterDocument(operation, definition *ast.Document) {
	s.operation = operation
	s.definition = definition
}

func (s *skipUserVisitor) EnterField(ref int) {
	if bytes.Equal(s.operation.FieldNameBytes(ref), []byte("user")) {
		s.SkipNode()
	}
}

func BenchmarkVisitor(b *testing.B) {

	definition := unsafeparser.ParseGraphqlDocumentString(testDefinition)
	operation := unsafeparser.ParseGraphqlDocumentString(testOperation)

	visitor := &dummyVisitor{}

	walker := astvisitor.NewWalker(48)
	walker.RegisterAllNodesVisitor(visitor)
	report := operationreport.Report{}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		report.Reset()
		walker.Walk(&operation, &definition, &report)
	}
}

func BenchmarkMinimalVisitor(b *testing.B) {

	definition := unsafeparser.ParseGraphqlDocumentString(testDefinition)
	operation := unsafeparser.ParseGraphqlDocumentString(testOperation)

	visitor := &minimalVisitor{}

	walker := astvisitor.NewWalker(48)
	walker.RegisterEnterFieldVisitor(visitor)
	walker.SetVisitorFilter(visitor)
	report := operationreport.Report{}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		report.Reset()
		walker.Walk(&operation, &definition, &report)
	}
}

type minimalVisitor struct {
}

func (m *minimalVisitor) AllowVisitor(kind astvisitor.VisitorKind, ref int, visitor interface{}, ancestorSkip astvisitor.SkipVisitors) bool {
	return visitor == m
}

func (m *minimalVisitor) EnterField(ref int) {

}

type dummyVisitor struct {
}

func (d *dummyVisitor) EnterSchemaExtension(ref int) {

}

func (d *dummyVisitor) LeaveSchemaExtension(ref int) {

}

func (d *dummyVisitor) EnterSchemaDefinition(ref int) {

}

func (d *dummyVisitor) LeaveSchemaDefinition(ref int) {

}

func (d *dummyVisitor) EnterRootOperationTypeDefinition(ref int) {

}

func (d *dummyVisitor) LeaveRootOperationTypeDefinition(ref int) {

}

func (d *dummyVisitor) LeaveDirectiveLocation(location ast.DirectiveLocation) {

}

func (d *dummyVisitor) EnterDirectiveDefinition(ref int) {

}

func (d *dummyVisitor) LeaveDirectiveDefinition(ref int) {

}

func (d *dummyVisitor) EnterDirectiveLocation(location ast.DirectiveLocation) {

}

func (d *dummyVisitor) EnterUnionMemberType(ref int) {

}

func (d *dummyVisitor) LeaveUnionMemberType(ref int) {

}

func (d *dummyVisitor) EnterObjectTypeDefinition(ref int) {

}

func (d *dummyVisitor) LeaveObjectTypeDefinition(ref int) {

}

func (d *dummyVisitor) EnterObjectTypeExtension(ref int) {

}

func (d *dummyVisitor) LeaveObjectTypeExtension(ref int) {

}

func (d *dummyVisitor) EnterFieldDefinition(ref int) {

}

func (d *dummyVisitor) LeaveFieldDefinition(ref int) {

}

func (d *dummyVisitor) EnterInputValueDefinition(ref int) {

}

func (d *dummyVisitor) LeaveInputValueDefinition(ref int) {

}

func (d *dummyVisitor) EnterInterfaceTypeDefinition(ref int) {

}

func (d *dummyVisitor) LeaveInterfaceTypeDefinition(ref int) {

}

func (d *dummyVisitor) EnterInterfaceTypeExtension(ref int) {

}

func (d *dummyVisitor) LeaveInterfaceTypeExtension(ref int) {

}

func (d *dummyVisitor) EnterScalarTypeDefinition(ref int) {

}

func (d *dummyVisitor) LeaveScalarTypeDefinition(ref int) {

}

func (d *dummyVisitor) EnterScalarTypeExtension(ref int) {

}

func (d *dummyVisitor) LeaveScalarTypeExtension(ref int) {

}

func (d *dummyVisitor) EnterUnionTypeDefinition(ref int) {

}

func (d *dummyVisitor) LeaveUnionTypeDefinition(ref int) {

}

func (d *dummyVisitor) EnterUnionTypeExtension(ref int) {

}

func (d *dummyVisitor) LeaveUnionTypeExtension(ref int) {

}

func (d *dummyVisitor) EnterEnumTypeDefinition(ref int) {

}

func (d *dummyVisitor) LeaveEnumTypeDefinition(ref int) {

}

func (d *dummyVisitor) EnterEnumTypeExtension(ref int) {

}

func (d *dummyVisitor) LeaveEnumTypeExtension(ref int) {

}

func (d *dummyVisitor) EnterEnumValueDefinition(ref int) {

}

func (d *dummyVisitor) LeaveEnumValueDefinition(ref int) {

}

func (d *dummyVisitor) EnterInputObjectTypeDefinition(ref int) {

}

func (d *dummyVisitor) LeaveInputObjectTypeDefinition(ref int) {

}

func (d *dummyVisitor) EnterInputObjectTypeExtension(ref int) {

}

func (d *dummyVisitor) LeaveInputObjectTypeExtension(ref int) {

}

func (d *dummyVisitor) EnterDocument(operation, definition *ast.Document) {

}

func (d *dummyVisitor) LeaveDocument(operation, definition *ast.Document) {

}

func (d *dummyVisitor) EnterDirective(ref int) {

}

func (d *dummyVisitor) LeaveDirective(ref int) {

}

func (d *dummyVisitor) EnterVariableDefinition(ref int) {

}

func (d *dummyVisitor) LeaveVariableDefinition(ref int) {

}

func (d *dummyVisitor) EnterOperationDefinition(ref int) {

}

func (d *dummyVisitor) LeaveOperationDefinition(ref int) {

}

func (d *dummyVisitor) EnterSelectionSet(ref int) {

}

func (d *dummyVisitor) LeaveSelectionSet(ref int) {

}

func (d *dummyVisitor) EnterField(ref int) {

}

func (d *dummyVisitor) LeaveField(ref int) {

}

func (d *dummyVisitor) EnterArgument(ref int) {

}

func (d *dummyVisitor) LeaveArgument(ref int) {

}

func (d *dummyVisitor) EnterFragmentSpread(ref int) {

}

func (d *dummyVisitor) LeaveFragmentSpread(ref int) {

}

func (d *dummyVisitor) EnterInlineFragment(ref int) {

}

func (d *dummyVisitor) LeaveInlineFragment(ref int) {

}

func (d *dummyVisitor) EnterFragmentDefinition(ref int) {

}

func (d *dummyVisitor) LeaveFragmentDefinition(ref int) {

}

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
}

func (p *printingVisitor) LeaveDocument(operation, definition *ast.Document) {

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
	parentTypeName := p.definition.NodeNameUnsafeString(p.EnclosingTypeDefinition)
	p.must(fmt.Fprintf(p.out, "EnterSelectionSet(%s): ref: %d\n", parentTypeName, ref))
}

func (p *printingVisitor) LeaveSelectionSet(ref int) {
	p.leave()
	parentTypeName := p.definition.NodeNameUnsafeString(p.EnclosingTypeDefinition)
	p.must(fmt.Fprintf(p.out, "LeaveSelectionSet(%s): ref: %d\n", parentTypeName, ref))
}

func (p *printingVisitor) EnterField(ref int) {
	p.enter()
	fieldName := p.operation.FieldNameBytes(ref)
	parentTypeName := p.definition.NodeNameUnsafeString(p.EnclosingTypeDefinition)
	p.must(fmt.Fprintf(p.out, "EnterField(%s::%s): ref: %d\n", fieldName, parentTypeName, ref))
}

func (p *printingVisitor) LeaveField(ref int) {
	p.leave()
	fieldName := p.operation.FieldNameUnsafeString(ref)
	parentTypeName := p.definition.NodeNameUnsafeString(p.EnclosingTypeDefinition)
	p.must(fmt.Fprintf(p.out, "LeaveField(%s::%s): ref: %d\n", fieldName, parentTypeName, ref))
}

func (p *printingVisitor) EnterArgument(ref int) {
	p.enter()
	argName := p.operation.ArgumentNameString(ref)
	parentTypeName := p.definition.NodeNameUnsafeString(p.EnclosingTypeDefinition)
	p.must(fmt.Fprintf(p.out, "EnterArgument(%s::%s): ref: %d\n", argName, parentTypeName, ref))
}

func (p *printingVisitor) LeaveArgument(ref int) {
	p.leave()
	argName := p.operation.ArgumentNameString(ref)
	parentTypeName := p.definition.NodeNameUnsafeString(p.EnclosingTypeDefinition)
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

const testDefinitions = `
directive @awesome on SCALAR | SCHEMA
type Foo {
	field: String
}
type FooBar {
	field: String
}
extend type Foo {
	field2: Boolean
}
interface Bar {
	field: String
}
extend interface Bar {
	field2: Boolean
}
enum Bat {
	BAR
}
extend enum Bat {
	BAL
}
union Fooniun = Foo
extend union Fooniun = FooBar
input Bart {
	field: String
}
extend input Bart {
	field2: Boolean
}
scalar JSON
extend scalar JSON @awesome
schema {
	query: Query
	mutation: Mutation
}
extend schema @awesome {}
extend schema {
	subscription: Subscription
}
`

const testOperation = `
query PostsUserQuery {
	posts {
		id
		description
		user {
			id
			name
		}
	}
}
fragment FirstFragment on Post {
	id
}
query ArgsQuery {
	foo(bar: "barValue", baz: true){
		fooField
	}
}
query VariableQuery($bar: String, $baz: Boolean) {
	foo(bar: $bar, baz: $baz){
		fooField
	}
}
query VariableQuery {
	posts {
		id @include(if: true)
	}
}
`

const testDefinition = `
directive @include(if: Boolean!) on FIELD | FRAGMENT_SPREAD | INLINE_FRAGMENT
schema {
	query: Query
}
type Query {
	posts: [Post]
	foo(bar: String!, baz: Boolean!): Foo
}
type User {
	id: ID
	name: String
	posts: [Post]
}
type Post {
	id: ID
	description: String
	user: User
}
type Foo {
	fooField: String
}
scalar ID
scalar String
`
