package execution

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type Planner struct {
	walker  *astvisitor.Walker
	visitor *planningVisitor
}

type ResolverDefinition struct {
	TypeName  []byte
	FieldName []byte
	Resolver  Resolver
}

type ResolverDefinitions []ResolverDefinition

func (r ResolverDefinitions) DefinitionForTypeField(typeName, fieldName []byte, definition *ResolverDefinition) (exists bool) {
	for i := 0; i < len(r); i++ {
		if bytes.Equal(typeName, r[i].TypeName) && bytes.Equal(fieldName, r[i].FieldName) {
			*definition = r[i]
			return true
		}
	}
	return false
}

func NewPlanner(resolverDefinitions ResolverDefinitions) *Planner {
	walker := astvisitor.NewWalker(48)
	visitor := planningVisitor{
		Walker:              &walker,
		resolverDefinitions: resolverDefinitions,
	}

	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterFieldVisitor(&visitor)
	walker.RegisterLeaveFieldVisitor(&visitor)
	walker.RegisterEnterOperationVisitor(&visitor)
	walker.RegisterEnterArgumentVisitor(&visitor)

	return &Planner{
		walker:  &walker,
		visitor: &visitor,
	}
}

func (p *Planner) Plan(operation, definition *ast.Document, report *operationreport.Report) Node {
	p.walker.Walk(operation, definition, report)
	return p.visitor.rootNode
}

type planningVisitor struct {
	*astvisitor.Walker
	resolverDefinitions   ResolverDefinitions
	operation, definition *ast.Document
	rootNode              Node
	currentNode           []Node
	currentResolve        *Resolve
}

func (p *planningVisitor) EnterDocument(operation, definition *ast.Document) {
	p.operation, p.definition = operation, definition
	obj := &Object{}
	p.rootNode = &Object{
		Fields: []Field{
			{
				Name:  literal.DATA,
				Value: obj,
			},
		},
	}
	p.currentNode = p.currentNode[:0]
	p.currentNode = append(p.currentNode, obj)
}

func (p *planningVisitor) EnterOperationDefinition(ref int) {

}

func (p *planningVisitor) EnterField(ref int) {

	definition, exists := p.FieldDefinition(ref)
	if !exists {
		return
	}

	resolverTypeName := p.definition.NodeResolverTypeName(p.EnclosingTypeDefinition, p.Path)

	var resolverDefinition ResolverDefinition
	hasResolverDefinition := p.resolverDefinitions.DefinitionForTypeField(resolverTypeName, p.operation.FieldNameBytes(ref), &resolverDefinition)
	if hasResolverDefinition {
		p.currentResolve = &Resolve{
			Resolver: resolverDefinition.Resolver,
		}
	} else {
		p.currentResolve = nil
	}

	switch parent := p.currentNode[len(p.currentNode)-1].(type) {
	case *Object:

		var value Node
		if p.definition.TypeIsList(p.definition.FieldDefinitionType(definition)) {
			obj := &Object{}
			list := &List{
				Path: []string{
					p.operation.FieldNameString(ref),
				},
				Value: obj,
			}

			parent.Fields = append(parent.Fields, Field{
				Name:    p.operation.FieldNameBytes(ref),
				Value:   list,
				Resolve: p.currentResolve,
			})

			p.currentNode = append(p.currentNode, obj)
			return
		}

		if !p.operation.FieldHasSelections(ref) {
			value = &Value{
				Path: []string{
					p.operation.FieldNameString(ref),
				},
			}
		} else {
			value = &Object{
				Path: []string{
					p.operation.FieldNameString(ref),
				},
			}
		}

		parent.Fields = append(parent.Fields, Field{
			Name:    p.operation.FieldNameBytes(ref),
			Value:   value,
			Resolve: p.currentResolve,
		})

		p.currentNode = append(p.currentNode, value)
	}
}

func (p *planningVisitor) LeaveField(ref int) {
	p.currentNode = p.currentNode[:len(p.currentNode)-1]
}

func (p *planningVisitor) EnterArgument(ref int) {
	if p.Ancestors[len(p.Ancestors)-1].Kind != ast.NodeKindField {
		return
	}
	if p.currentResolve == nil {
		return
	}

	name := p.operation.ArgumentNameBytes(ref)
	value := p.operation.ArgumentValue(ref)

	if value.Kind == ast.ValueKindVariable {
		p.currentResolve.Args = append(p.currentResolve.Args, &ContextVariableArgument{
			Name:         name,
			VariableName: p.operation.VariableValueNameBytes(value.Ref),
		})
	}
}
