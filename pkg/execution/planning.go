package execution

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
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
	walker.RegisterEnterSelectionSetVisitor(&visitor)
	walker.RegisterLeaveSelectionSetVisitor(&visitor)

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
	currentResolve        currentResolve
}

type resolveRef struct {
	path        ast.Path
	fieldRef    int
	resolve     *Resolve
	document    *ast.Document
	currentNode []ast.Node
}

type currentResolve []*resolveRef

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
		doc := ast.NewDocument()
		field := ast.Field{
			Name: doc.Input.AppendInputBytes(p.operation.FieldNameBytes(ref)),
		}
		doc.Fields = append(doc.Fields, field)
		fieldRef := len(doc.Fields) - 1
		selection := ast.Selection{
			Kind: ast.SelectionKindField,
			Ref:  fieldRef,
		}
		doc.Selections = append(doc.Selections, selection)
		selectionRef := len(doc.Selections) - 1
		set := ast.SelectionSet{
			SelectionRefs: []int{selectionRef},
		}
		doc.SelectionSets = append(doc.SelectionSets, set)
		setRef := len(doc.SelectionSets) - 1
		operationDefinition := ast.OperationDefinition{
			Name:          doc.Input.AppendInputBytes([]byte("o")),
			OperationType: p.operation.OperationDefinitions[p.Ancestors[0].Ref].OperationType,
			SelectionSet:  setRef,
			HasSelections: true,
		}
		doc.OperationDefinitions = append(doc.OperationDefinitions, operationDefinition)
		operationDefinitionRef := len(doc.OperationDefinitions) - 1
		doc.RootNodes = append(doc.RootNodes, ast.Node{
			Kind: ast.NodeKindOperationDefinition,
			Ref:  operationDefinitionRef,
		})
		resolve := &resolveRef{
			resolve: &Resolve{
				Resolver: resolverDefinition.Resolver,
			},
			path:     p.Path,
			fieldRef: ref,
			document: doc,
		}
		resolve.currentNode = append(resolve.currentNode, ast.Node{
			Kind: ast.NodeKindOperationDefinition,
			Ref:  operationDefinitionRef,
		})
		resolve.currentNode = append(resolve.currentNode, ast.Node{
			Kind: ast.NodeKindSelectionSet,
			Ref:  setRef,
		})
		resolve.currentNode = append(resolve.currentNode, ast.Node{
			Kind: ast.NodeKindField,
			Ref:  fieldRef,
		})
		p.currentResolve = append(p.currentResolve, resolve)
	} else {
		resolve := p.currentResolve[len(p.currentResolve)-1]
		field := ast.Field{
			Name: resolve.document.Input.AppendInputBytes(p.operation.FieldNameBytes(ref)),
		}
		resolve.document.Fields = append(resolve.document.Fields, field)
		fieldRef := len(resolve.document.Fields) - 1
		set := resolve.currentNode[len(resolve.currentNode)-1]
		selection := ast.Selection{
			Kind: ast.SelectionKindField,
			Ref:  fieldRef,
		}
		resolve.document.Selections = append(resolve.document.Selections, selection)
		selectionRef := len(resolve.document.Selections) - 1
		resolve.document.SelectionSets[set.Ref].SelectionRefs = append(resolve.document.SelectionSets[set.Ref].SelectionRefs, selectionRef)
		resolve.currentNode = append(resolve.currentNode, ast.Node{
			Kind: ast.NodeKindField,
			Ref:  fieldRef,
		})
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

			var resolve *Resolve
			resolveRef := p.currentResolve[len(p.currentResolve)-1]
			if resolveRef.path.Equals(p.Path) && resolveRef.fieldRef == ref {
				resolve = resolveRef.resolve
			}

			parent.Fields = append(parent.Fields, Field{
				Name:    p.operation.FieldNameBytes(ref),
				Value:   list,
				Resolve: resolve,
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

		var resolve *Resolve
		resolveRef := p.currentResolve[len(p.currentResolve)-1]
		if resolveRef.path.Equals(p.Path) && resolveRef.fieldRef == ref {
			resolve = resolveRef.resolve
		}

		parent.Fields = append(parent.Fields, Field{
			Name:    p.operation.FieldNameBytes(ref),
			Value:   value,
			Resolve: resolve,
		})

		p.currentNode = append(p.currentNode, value)
	}
}

func (p *planningVisitor) LeaveField(ref int) {
	resolve := p.currentResolve[len(p.currentResolve)-1]
	if resolve.path.Equals(p.Path) && resolve.fieldRef == ref {
		switch resolve.resolve.Resolver.(type) {
		case *GraphQLResolver:
			buff := bytes.Buffer{}
			err := astprinter.Print(resolve.document, nil, &buff)
			if err != nil {
				p.StopWithInternalErr(err)
				return
			}
			arg := &StaticVariableArgument{
				Name:  literal.QUERY,
				Value: buff.Bytes(),
			}
			resolve.resolve.Args = append([]Argument{arg}, resolve.resolve.Args...)
		}
		p.currentResolve = p.currentResolve[:len(p.currentResolve)-1]
	} else {
		resolve.currentNode = resolve.currentNode[:len(resolve.currentNode)-1]
	}
	p.currentNode = p.currentNode[:len(p.currentNode)-1]
}

func (p *planningVisitor) EnterArgument(ref int) {

	resolve := p.currentResolve[len(p.currentResolve)-1]

	name := p.operation.ArgumentNameBytes(ref)
	value := p.operation.ArgumentValue(ref)

	if value.Kind == ast.ValueKindVariable {

		resolve.resolve.Args = append(resolve.resolve.Args, &ContextVariableArgument{
			Name:         name,
			VariableName: p.operation.VariableValueNameBytes(value.Ref),
		})

		variableValue := ast.VariableValue{
			Name: resolve.document.Input.AppendInputBytes(p.operation.VariableValueNameBytes(value.Ref)),
		}
		resolve.document.VariableValues = append(resolve.document.VariableValues, variableValue)
		variableRef := len(resolve.document.VariableValues) - 1
		arg := ast.Argument{
			Name: resolve.document.Input.AppendInputBytes(p.operation.ArgumentNameBytes(ref)),
			Value: ast.Value{
				Kind: ast.ValueKindVariable,
				Ref:  variableRef,
			},
		}
		resolve.document.Arguments = append(resolve.document.Arguments, arg)
		argRef := len(resolve.document.Arguments) - 1
		fieldRef := resolve.currentNode[len(resolve.currentNode)-1].Ref
		resolve.document.Fields[fieldRef].HasArguments = true
		resolve.document.Fields[fieldRef].Arguments.Refs = append(resolve.document.Fields[fieldRef].Arguments.Refs, argRef)

		inputValueDefinitionRef, exists := p.ArgumentInputValueDefinition(ref)
		if !exists {
			return
		}

		typeRef := p.definition.InputValueDefinitionType(inputValueDefinitionRef)

		variableDefinition := ast.VariableDefinition{
			VariableValue: ast.Value{
				Kind: ast.ValueKindVariable,
				Ref:  variableRef,
			},
			Type: resolve.document.ImportType(typeRef, p.definition),
		}
		resolve.document.VariableDefinitions = append(resolve.document.VariableDefinitions, variableDefinition)
		variableDefinitionRef := len(resolve.document.VariableDefinitions) - 1
		operationRef := resolve.document.RootNodes[0].Ref
		resolve.document.OperationDefinitions[operationRef].HasVariableDefinitions = true
		resolve.document.OperationDefinitions[operationRef].VariableDefinitions.Refs = append(resolve.document.OperationDefinitions[operationRef].VariableDefinitions.Refs, variableDefinitionRef)
	}
}

func (p *planningVisitor) EnterSelectionSet(ref int) {
	if len(p.currentResolve) == 0 {
		return
	}
	resolve := p.currentResolve[len(p.currentResolve)-1]

	field := resolve.currentNode[len(resolve.currentNode)-1]

	set := ast.SelectionSet{}
	resolve.document.SelectionSets = append(resolve.document.SelectionSets, set)
	setRef := len(resolve.document.SelectionSets) - 1

	resolve.document.Fields[field.Ref].HasSelections = true
	resolve.document.Fields[field.Ref].SelectionSet = setRef

	resolve.currentNode = append(resolve.currentNode, ast.Node{
		Kind: ast.NodeKindSelectionSet,
		Ref:  setRef,
	})
}

func (p *planningVisitor) LeaveSelectionSet(ref int) {
	if len(p.currentResolve) == 0 {
		return
	}
	resolve := p.currentResolve[len(p.currentResolve)-1]
	resolve.currentNode = resolve.currentNode[:len(resolve.currentNode)-1]
}
