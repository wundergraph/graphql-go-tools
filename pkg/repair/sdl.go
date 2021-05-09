package repair

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

func SDL(input string) (string, error) {
	repair := sdlRepair{
		sdl: input,
	}
	return repair.do()
}

type sdlRepair struct {
	sdl string
	doc *ast.Document
}

func (r *sdlRepair) do() (string, error) {
	doc, report := astparser.ParseGraphqlDocumentString(r.sdl)
	if report.HasErrors() {
		return "", report
	}
	r.doc = &doc
	err := r.repairEmptyInputObjectTypeDefinitions()
	if err != nil {
		return "", err
	}
	return astprinter.PrintString(r.doc,nil)
}

func (r *sdlRepair) repairEmptyInputObjectTypeDefinitions () error {
	walker := astvisitor.NewWalker(8)
	visitor := &emptyInputObjectTypeDefinitionVisitor{
		walker: &walker,
	}
	walker.RegisterEnterInputObjectTypeDefinitionVisitor(visitor)
	walker.RegisterEnterInputValueDefinitionVisitor(visitor)
	walker.RegisterDocumentVisitor(visitor)
	report := operationreport.Report{}
	for {
		walker.Walk(r.doc,nil,&report)
		if report.HasErrors() {
			return report
		}
		if visitor.changed {
			continue
		}
		return nil
	}
}

type emptyInputObjectTypeDefinitionVisitor struct {
	walker *astvisitor.Walker
	changed bool
	doc *ast.Document

	removeRootNode bool
	rootNode ast.Node

	removeFieldsWithType []string

	removeInputValueDefinition bool
	inputObjectTypeDefinition int
	inputValueDefinition int

	removeFieldArgument bool
	fieldDefinition int
}

func (e *emptyInputObjectTypeDefinitionVisitor) EnterInputValueDefinition(ref int) {
	fieldType := e.doc.InputValueDefinitionType(ref)
	typeName := e.doc.ResolveTypeNameString(fieldType)
	for _, s := range e.removeFieldsWithType {
		if typeName == s {
			ancestor := e.walker.Ancestors[len(e.walker.Ancestors)-1]
			switch ancestor.Kind {
			case ast.NodeKindInputObjectTypeDefinition:
				e.changed = true
				e.removeInputValueDefinition = true
				e.inputObjectTypeDefinition = ancestor.Ref
				e.inputValueDefinition = ref
				return
			case ast.NodeKindFieldDefinition:
				e.changed = true
				e.removeFieldArgument = true
				e.fieldDefinition = ancestor.Ref
				e.inputValueDefinition = ref
			}
		}
	}
}

func (e *emptyInputObjectTypeDefinitionVisitor) LeaveDocument(operation, definition *ast.Document) {
	if e.removeRootNode {
		e.doc.RemoveRootNode(e.rootNode)
	}
	if e.removeInputValueDefinition {
		for i, ref := range e.doc.InputObjectTypeDefinitions[e.inputObjectTypeDefinition].InputFieldsDefinition.Refs {
			if ref == e.inputValueDefinition {
				e.doc.InputObjectTypeDefinitions[e.inputObjectTypeDefinition].InputFieldsDefinition.Refs =
					append(e.doc.InputObjectTypeDefinitions[e.inputObjectTypeDefinition].InputFieldsDefinition.Refs[:i],e.doc.InputObjectTypeDefinitions[e.inputObjectTypeDefinition].InputFieldsDefinition.Refs[i+1:]...)
				e.doc.InputObjectTypeDefinitions[e.inputObjectTypeDefinition].HasInputFieldsDefinition = len(e.doc.InputObjectTypeDefinitions[e.inputObjectTypeDefinition].InputFieldsDefinition.Refs) != 0
				return
			}
		}
	}
	if e.removeFieldArgument {
		for i, ref := range e.doc.FieldDefinitions[e.fieldDefinition].ArgumentsDefinition.Refs {
			if ref == e.inputValueDefinition {
				e.doc.FieldDefinitions[e.fieldDefinition].ArgumentsDefinition.Refs =
					append(e.doc.FieldDefinitions[e.fieldDefinition].ArgumentsDefinition.Refs[:i],
						e.doc.FieldDefinitions[e.fieldDefinition].ArgumentsDefinition.Refs[i+1:]...)
				e.doc.FieldDefinitions[e.fieldDefinition].HasArgumentsDefinitions = len(e.doc.FieldDefinitions[e.fieldDefinition].ArgumentsDefinition.Refs) != 0
			}
		}
	}
}

func (e *emptyInputObjectTypeDefinitionVisitor) EnterDocument(operation, definition *ast.Document) {
	e.doc = operation
	e.changed = false
	e.removeInputValueDefinition = false
}

func (e *emptyInputObjectTypeDefinitionVisitor) EnterInputObjectTypeDefinition(ref int) {
	if e.doc.InputObjectTypeDefinitions[ref].HasInputFieldsDefinition{
		return
	}
	e.changed = true
	for _, node := range e.doc.RootNodes {
		if node.Kind != ast.NodeKindInputObjectTypeDefinition || node.Ref != ref {
			continue
		}
		e.removeRootNode = true
		e.rootNode = node
		e.removeFieldsWithType = append(e.removeFieldsWithType, node.NameString(e.doc))
		return
	}
}

