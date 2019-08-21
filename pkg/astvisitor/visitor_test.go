package astvisitor

import (
	"bytes"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/sebdah/goldie"
	"io"
	"testing"
)

var must = func(err error) {
	if err != nil {
		panic(err)
	}
}

var mustDoc = func(doc *ast.Document, err error) *ast.Document {
	must(err)
	return doc
}

func TestVisit(t *testing.T) {

	definition := mustDoc(astparser.ParseGraphqlDocumentString(testDefinition))
	operation := mustDoc(astparser.ParseGraphqlDocumentString(testOperation))

	walker := NewWalker(48)
	buff := &bytes.Buffer{}
	visitor := &printingVisitor{
		out:        buff,
		operation:  operation,
		definition: definition,
	}

	walker.RegisterAllNodesVisitor(visitor)

	must(walker.Walk(operation, definition))

	goldie.Assert(t, "visitor", buff.Bytes())
}

func BenchmarkVisitor(b *testing.B) {

	definition := mustDoc(astparser.ParseGraphqlDocumentString(testDefinition))
	operation := mustDoc(astparser.ParseGraphqlDocumentString(testOperation))

	visitor := &dummyVisitor{}

	walker := NewWalker(48)
	walker.RegisterAllNodesVisitor(visitor)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		walker.Walk(operation, definition)
	}
}

func BenchmarkMinimalVisitor(b *testing.B) {

	definition := mustDoc(astparser.ParseGraphqlDocumentString(testDefinition))
	operation := mustDoc(astparser.ParseGraphqlDocumentString(testOperation))

	visitor := &minimalVisitor{}

	walker := NewWalker(48)
	walker.RegisterEnterFieldVisitor(visitor)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		walker.Walk(operation, definition)
	}
}

type minimalVisitor struct {
}

func (m *minimalVisitor) EnterField(ref int, info Info) Instruction {
	return Instruction{}
}

type dummyVisitor struct {
}

func (d *dummyVisitor) EnterOperationDefinition(ref int, info Info) Instruction {
	return Instruction{}
}

func (d *dummyVisitor) LeaveOperationDefinition(ref int, info Info) Instruction {
	return Instruction{}
}

func (d *dummyVisitor) EnterSelectionSet(ref int, info Info) Instruction {
	return Instruction{}
}

func (d *dummyVisitor) LeaveSelectionSet(ref int, info Info) Instruction {
	return Instruction{}
}

func (d *dummyVisitor) EnterField(ref int, info Info) Instruction {
	return Instruction{}
}

func (d *dummyVisitor) LeaveField(ref int, info Info) Instruction {
	return Instruction{}
}

func (d *dummyVisitor) EnterArgument(ref int, definition int, info Info) Instruction {
	return Instruction{}
}

func (d *dummyVisitor) LeaveArgument(ref int, definition int, info Info) Instruction {
	return Instruction{}
}

func (d *dummyVisitor) EnterFragmentSpread(ref int, info Info) Instruction {
	return Instruction{}
}

func (d *dummyVisitor) LeaveFragmentSpread(ref int, info Info) Instruction {
	return Instruction{}
}

func (d *dummyVisitor) EnterInlineFragment(ref int, info Info) Instruction {
	return Instruction{}
}

func (d *dummyVisitor) LeaveInlineFragment(ref int, info Info) Instruction {
	return Instruction{}
}

func (d *dummyVisitor) EnterFragmentDefinition(ref int, info Info) Instruction {
	return Instruction{}
}

func (d *dummyVisitor) LeaveFragmentDefinition(ref int, info Info) Instruction {
	return Instruction{}
}

type printingVisitor struct {
	out         io.Writer
	operation   *ast.Document
	definition  *ast.Document
	indentation int
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

func (p *printingVisitor) printSelections(info Info) (out string) {
	out += "SelectionsBefore: " + p.operation.PrintSelections(info.SelectionsBefore)
	out += " SelectionsAfter: " + p.operation.PrintSelections(info.SelectionsAfter)
	return
}

func (p *printingVisitor) EnterOperationDefinition(ref int, info Info) Instruction {
	p.enter()
	name := p.operation.Input.ByteSliceString(p.operation.OperationDefinitions[ref].Name)
	if name == "" {
		name = "anonymous!"
	}
	p.must(fmt.Fprintf(p.out, "EnterOperationDefinition (%s): ref: %d, info: %+v\n", name, ref, info))
	return Instruction{}
}

func (p *printingVisitor) LeaveOperationDefinition(ref int, info Info) Instruction {
	p.leave()
	p.must(fmt.Fprintf(p.out, "LeaveOperationDefinition: ref: %d, info: %+v\n\n", ref, info))
	return Instruction{}
}

func (p *printingVisitor) EnterSelectionSet(ref int, info Info) Instruction {
	p.enter()
	parentTypeName := p.definition.NodeTypeNameString(info.EnclosingTypeDefinition)
	p.must(fmt.Fprintf(p.out, "EnterSelectionSet(%s): ref: %d, info: %+v\n", parentTypeName, ref, info))
	return Instruction{}
}

func (p *printingVisitor) LeaveSelectionSet(ref int, info Info) Instruction {
	p.leave()
	parentTypeName := p.definition.NodeTypeNameString(info.EnclosingTypeDefinition)
	p.must(fmt.Fprintf(p.out, "LeaveSelectionSet(%s): ref: %d, info: %+v\n", parentTypeName, ref, info))
	return Instruction{}
}

func (p *printingVisitor) EnterField(ref int, info Info) Instruction {
	p.enter()
	fieldName := p.operation.FieldName(ref)
	parentTypeName := p.definition.NodeTypeNameString(info.EnclosingTypeDefinition)
	p.must(fmt.Fprintf(p.out, "EnterField(%s::%s): ref: %d, info: %+v, %s\n", fieldName, parentTypeName, ref, info, p.printSelections(info)))
	return Instruction{}
}

func (p *printingVisitor) LeaveField(ref int, info Info) Instruction {
	p.leave()
	fieldName := p.operation.FieldNameString(ref)
	parentTypeName := p.definition.NodeTypeNameString(info.EnclosingTypeDefinition)
	p.must(fmt.Fprintf(p.out, "LeaveField(%s::%s): ref: %d, info: %+v\n", fieldName, parentTypeName, ref, info))
	return Instruction{}
}

func (p *printingVisitor) EnterArgument(ref int, definition int, info Info) Instruction {
	p.enter()
	argName := p.operation.ArgumentNameString(ref)
	parentTypeName := p.definition.NodeTypeNameString(info.EnclosingTypeDefinition)
	def := p.definition.InputValueDefinitions[definition]
	p.must(fmt.Fprintf(p.out, "EnterArgument(%s::%s): ref: %d, definition: %+v, info: %+v\n", argName, parentTypeName, ref, def, info))
	return Instruction{}
}

func (p *printingVisitor) LeaveArgument(ref int, definition int, info Info) Instruction {
	p.leave()
	argName := p.operation.ArgumentNameString(ref)
	parentTypeName := p.definition.NodeTypeNameString(info.EnclosingTypeDefinition)
	def := p.definition.InputValueDefinitions[definition]
	p.must(fmt.Fprintf(p.out, "LeaveArgument(%s::%s): ref: %d,definition: %+v, info: %+v\n", argName, parentTypeName, ref, def, info))
	return Instruction{}
}

func (p *printingVisitor) EnterFragmentSpread(ref int, info Info) Instruction {
	p.enter()
	spreadName := p.operation.FragmentSpreadNameString(ref)
	p.must(fmt.Fprintf(p.out, "EnterFragmentSpread(%s): ref: %d, info: %+v\n", spreadName, ref, info))
	return Instruction{}
}

func (p *printingVisitor) LeaveFragmentSpread(ref int, info Info) Instruction {
	p.leave()
	spreadName := p.operation.FragmentSpreadNameString(ref)
	p.must(fmt.Fprintf(p.out, "LeaveFragmentSpread(%s): ref: %d, info: %+v\n", spreadName, ref, info))
	return Instruction{}
}

func (p *printingVisitor) EnterInlineFragment(ref int, info Info) Instruction {
	p.enter()
	typeConditionName := p.operation.InlineFragmentTypeConditionNameString(ref)
	if typeConditionName == "" {
		typeConditionName = "anonymous!"
	}
	p.must(fmt.Fprintf(p.out, "EnterInlineFragment(%s): ref: %d, info: %+v\n", typeConditionName, ref, info))
	return Instruction{}
}

func (p *printingVisitor) LeaveInlineFragment(ref int, info Info) Instruction {
	p.leave()
	typeConditionName := p.operation.InlineFragmentTypeConditionNameString(ref)
	if typeConditionName == "" {
		typeConditionName = "anonymous!"
	}
	p.must(fmt.Fprintf(p.out, "LeaveInlineFragment(%s): ref: %d, info: %+v\n", typeConditionName, ref, info))
	return Instruction{}
}

func (p *printingVisitor) EnterFragmentDefinition(ref int, info Info) Instruction {
	p.enter()
	name := p.operation.FragmentDefinitionNameString(ref)
	p.must(fmt.Fprintf(p.out, "EnterFragmentDefinition(%s): ref: %d, info: %+v\n", name, ref, info))
	return Instruction{}
}

func (p *printingVisitor) LeaveFragmentDefinition(ref int, info Info) Instruction {
	p.leave()
	name := p.operation.FragmentDefinitionNameString(ref)
	p.must(fmt.Fprintf(p.out, "LeaveFragmentDefinition(%s): ref: %d, info: %+v\n\n", name, ref, info))
	return Instruction{}
}

const testOperation = `
query postsQuery {
	posts {
		id
		description
		...FirstFragment
		... {
			description
			... on Post {
				id
			}
		}
	}
}
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
`

const testDefinition = `
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
