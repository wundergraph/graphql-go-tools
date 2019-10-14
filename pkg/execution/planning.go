package execution

import (
	"bytes"
	"fmt"
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
	walker.RegisterEnterSelectionSetVisitor(&visitor)
	walker.RegisterLeaveSelectionSetVisitor(&visitor)
	walker.RegisterEnterInlineFragmentVisitor(&visitor)
	walker.RegisterLeaveInlineFragmentVisitor(&visitor)

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

func (p *planningVisitor) EnterInlineFragment(ref int) {
	resolve := p.currentResolve[len(p.currentResolve)-1]
	switch resolve.resolve.Resolver.(type) {
	case *GraphQLResolver:
		current := resolve.currentNode[len(resolve.currentNode)-1]
		if current.Kind != ast.NodeKindSelectionSet {
			return
		}
		inlineFragmentType := resolve.document.ImportType(p.operation.InlineFragments[ref].TypeCondition.Type, p.operation)
		resolve.document.InlineFragments = append(resolve.document.InlineFragments, ast.InlineFragment{
			TypeCondition: ast.TypeCondition{
				Type: inlineFragmentType,
			},
			SelectionSet: -1,
		})
		inlineFragmentRef := len(resolve.document.InlineFragments) - 1
		resolve.document.Selections = append(resolve.document.Selections, ast.Selection{
			Kind: ast.SelectionKindInlineFragment,
			Ref:  inlineFragmentRef,
		})
		selectionRef := len(resolve.document.Selections) - 1
		resolve.document.SelectionSets[current.Ref].SelectionRefs = append(resolve.document.SelectionSets[current.Ref].SelectionRefs, selectionRef)
		resolve.currentNode = append(resolve.currentNode, ast.Node{
			Kind: ast.NodeKindInlineFragment,
			Ref:  inlineFragmentRef,
		})
	}
}

func (p *planningVisitor) LeaveInlineFragment(ref int) {
	resolve := p.currentResolve[len(p.currentResolve)-1]
	switch resolve.resolve.Resolver.(type) {
	case *GraphQLResolver:
		resolve.currentNode = resolve.currentNode[:len(resolve.currentNode)-1]
	}
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
		params := p.resolverDirectiveParamObjectValues(ref, resolverDefinition)
		args := make([]int, len(params))
		variableDefinitions := make([]int, len(params))
		var resolveArgs []Argument
		if len(params) != 0 {
			resolveArgs = make([]Argument, 0, len(params))
		}
		for i := 0; i < len(params); i++ {
			doc.VariableValues = append(doc.VariableValues, ast.VariableValue{
				Name: doc.Input.AppendInputBytes(params[i].sourceName),
			})
			variableRef := len(doc.VariableValues) - 1
			variableValue := ast.Value{
				Kind: ast.ValueKindVariable,
				Ref:  variableRef,
			}
			doc.Arguments = append(doc.Arguments, ast.Argument{
				Name:  doc.Input.AppendInputBytes(params[i].name),
				Value: variableValue,
			})
			args[i] = len(doc.Arguments) - 1

			doc.Types = append(doc.Types, ast.Type{
				TypeKind: ast.TypeKindNamed,
				Name:     doc.Input.AppendInputBytes([]byte("String")),
				OfType:   -1,
			})

			stringTypeRef := len(doc.Types) - 1
			doc.Types = append(doc.Types, ast.Type{
				TypeKind: ast.TypeKindNonNull,
				OfType:   stringTypeRef,
			})

			nonNullTypeRef := len(doc.Types) - 1

			doc.VariableDefinitions = append(doc.VariableDefinitions, ast.VariableDefinition{
				VariableValue: variableValue,
				Type:          nonNullTypeRef,
			})
			variableDefinitions[i] = len(doc.VariableDefinitions) - 1

			switch {
			case bytes.Equal(params[i].sourceKind, []byte("CONTEXT_VARIABLE")):
				resolveArgs = append(resolveArgs, &ContextVariableArgument{
					Name:         params[i].name,
					VariableName: params[i].sourceName,
				})
			case bytes.Equal(params[i].sourceKind, []byte("OBJECT_VARIABLE_ARGUMENT")):
				resolveArgs = append(resolveArgs, &ObjectVariableArgument{
					Name: params[i].name,
					Path: []string{string(params[i].sourceName)},
				})
			case bytes.Equal(params[i].sourceKind, []byte("FIELD_ARGUMENTS")):
				arg, exists := p.operation.FieldArgument(ref, params[i].sourceName)
				if !exists {
					return
				}
				value := p.operation.ArgumentValue(arg)
				if value.Kind != ast.ValueKindVariable {
					return
				}
				variableName := p.operation.VariableValueNameBytes(value.Ref)
				resolveArgs = append(resolveArgs, &ContextVariableArgument{
					Name:         params[i].sourceName,
					VariableName: variableName,
				})
			}
		}

		switch dataSource := resolverDefinition.Resolver.(type) {
		case *StaticDataSource:
			fieldDefinition, ok := p.FieldDefinition(ref)
			if !ok {
				return
			}
			directive, ok := p.definition.FieldDefinitionDirectiveByName(fieldDefinition, dataSource.DirectiveName())
			if !ok {
				return
			}
			var staticValue []byte
			value, ok := p.definition.DirectiveArgumentValueByName(directive, []byte("data"))
			if !ok || value.Kind != ast.ValueKindString {
				staticValue = literal.NULL
			} else {
				staticValue = p.definition.StringValueContentBytes(value.Ref)
			}
			staticValue = bytes.ReplaceAll(staticValue, literal.BACKSLASH, nil)
			resolveArgs = append(resolveArgs, &StaticVariableArgument{
				Value: staticValue,
			})
		}

		field := ast.Field{
			Name: doc.Input.AppendInputBytes(p.resolverDirectiveFieldName(ref, resolverDefinition)),
			Arguments: ast.ArgumentList{
				Refs: args,
			},
			HasArguments: len(args) != 0,
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
			VariableDefinitions: ast.VariableDefinitionList{
				Refs: variableDefinitions,
			},
			HasVariableDefinitions: len(variableDefinitions) != 0,
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
				Args:     resolveArgs,
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
		if len(p.currentResolve) == 0 {
			p.StopWithInternalErr(fmt.Errorf("resolver missing for field: %s on Path: %s", p.operation.FieldNameString(ref), p.Path.String()))
			return
		}
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

		var resolve *Resolve
		resolveRef := p.currentResolve[len(p.currentResolve)-1]
		if resolveRef.path.Equals(p.Path) && resolveRef.fieldRef == ref {
			resolve = resolveRef.resolve
		}

		path := p.fieldPath(ref)
		if hasResolverDefinition {
			fieldName := p.resolverDirectiveFieldName(ref, resolverDefinition)
			if len(fieldName) != 0 {
				path[0] = string(fieldName)
			}
		}
		if resolve != nil {
			switch resolve.Resolver.(type) {
			case *RESTResolver:
				path = nil
			case *StaticDataSource:
				path = nil
			}
		}

		var value Node
		fieldDefinitionType := p.definition.FieldDefinitionType(definition)
		if p.definition.TypeIsList(fieldDefinitionType) {

			if !p.operation.FieldHasSelections(ref) {
				value = &Value{
					QuoteValue: p.quoteValue(fieldDefinitionType),
				}
			} else {
				value = &Object{}
			}

			list := &List{
				Path:  path,
				Value: value,
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

			p.currentNode = append(p.currentNode, value)
			return
		}

		if !p.operation.FieldHasSelections(ref) {
			value = &Value{
				Path:       path,
				QuoteValue: p.quoteValue(fieldDefinitionType),
			}
		} else {
			value = &Object{
				Path: path,
			}
		}

		var skipCondition BooleanCondition
		ancestor := p.Ancestors[len(p.Ancestors)-2]
		if ancestor.Kind == ast.NodeKindInlineFragment {
			typeConditionName := p.operation.InlineFragmentTypeConditionName(ancestor.Ref)
			skipCondition = &IfNotEqual{
				Left: &ObjectVariableArgument{
					Path: []string{"__typename"},
				},
				Right: &StaticVariableArgument{
					Value: typeConditionName,
				},
			}
		}

		parent.Fields = append(parent.Fields, Field{
			Name:    p.operation.FieldObjectNameBytes(ref),
			Value:   value,
			Resolve: resolve,
			Skip:    skipCondition,
		})

		p.currentNode = append(p.currentNode, value)
	}
}

func (p *planningVisitor) LeaveField(ref int) {
	resolve := p.currentResolve[len(p.currentResolve)-1]
	if resolve.path.Equals(p.Path) && resolve.fieldRef == ref {
		switch resolve.resolve.Resolver.(type) {
		case *RESTResolver:
			definition, exists := p.FieldDefinition(ref)
			if !exists {
				return
			}
			directive, exists := p.definition.FieldDefinitionDirectiveByName(definition, []byte("RESTDataSource"))
			if !exists {
				return
			}
			value, exists := p.definition.DirectiveArgumentValueByName(directive, literal.URL)
			if !exists {
				return
			}
			variableValue := p.definition.StringValueContentBytes(value.Ref)
			arg := &StaticVariableArgument{
				Name:  literal.URL,
				Value: variableValue,
			}
			resolve.resolve.Args = append([]Argument{arg}, resolve.resolve.Args...)
			value, exists = p.definition.DirectiveArgumentValueByName(directive, literal.HOST)
			if !exists {
				return
			}
			variableValue = p.definition.StringValueContentBytes(value.Ref)
			arg = &StaticVariableArgument{
				Name:  literal.HOST,
				Value: variableValue,
			}
			resolve.resolve.Args = append([]Argument{arg}, resolve.resolve.Args...)

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

			definition, exists := p.FieldDefinition(ref)
			if !exists {
				return
			}
			directive, exists := p.definition.FieldDefinitionDirectiveByName(definition, []byte("GraphQLDataSource"))
			if !exists {
				return
			}
			value, exists := p.definition.DirectiveArgumentValueByName(directive, literal.URL)
			if !exists {
				return
			}
			variableValue := p.definition.StringValueContentBytes(value.Ref)
			arg = &StaticVariableArgument{
				Name:  literal.URL,
				Value: variableValue,
			}
			resolve.resolve.Args = append([]Argument{arg}, resolve.resolve.Args...)
			value, exists = p.definition.DirectiveArgumentValueByName(directive, literal.HOST)
			if !exists {
				return
			}
			variableValue = p.definition.StringValueContentBytes(value.Ref)
			arg = &StaticVariableArgument{
				Name:  literal.HOST,
				Value: variableValue,
			}
			resolve.resolve.Args = append([]Argument{arg}, resolve.resolve.Args...)
		}
		p.currentResolve = p.currentResolve[:len(p.currentResolve)-1]
	} else {
		resolve.currentNode = resolve.currentNode[:len(resolve.currentNode)-1]
	}
	p.currentNode = p.currentNode[:len(p.currentNode)-1]
}

func (p *planningVisitor) EnterSelectionSet(ref int) {
	if len(p.currentResolve) == 0 {
		return
	}

	resolve := p.currentResolve[len(p.currentResolve)-1]
	fieldOrInlineFragment := resolve.currentNode[len(resolve.currentNode)-1]

	set := ast.SelectionSet{}
	resolve.document.SelectionSets = append(resolve.document.SelectionSets, set)
	setRef := len(resolve.document.SelectionSets) - 1

	switch fieldOrInlineFragment.Kind {
	case ast.NodeKindField:
		resolve.document.Fields[fieldOrInlineFragment.Ref].HasSelections = true
		resolve.document.Fields[fieldOrInlineFragment.Ref].SelectionSet = setRef
	case ast.NodeKindInlineFragment:
		resolve.document.InlineFragments[fieldOrInlineFragment.Ref].HasSelections = true
		resolve.document.InlineFragments[fieldOrInlineFragment.Ref].SelectionSet = setRef
	}

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

func (p *planningVisitor) resolverDirectiveParamObjectValues(field int, resolverDefinition ResolverDefinition) []ResolverParameter {
	definition, exists := p.FieldDefinition(field)
	if !exists {
		return nil
	}

	directive, exists := p.definition.FieldDefinitionDirectiveByName(definition, resolverDefinition.Resolver.DirectiveName())
	if !exists {
		return nil
	}

	paramsList, exists := p.definition.DirectiveArgumentValueByName(directive, []byte("params"))
	if !exists {
		return nil
	}

	if paramsList.Kind != ast.ValueKindList {
		return nil
	}

	objectValues := p.definition.ListValues[paramsList.Ref].Refs
	params := make([]ResolverParameter, len(objectValues))
	for i := 0; i < len(objectValues); i++ {
		value := p.definition.Value(objectValues[i])
		if value.Kind != ast.ValueKindObject {
			return nil
		}
		objectValue := p.definition.ObjectValues[value.Ref]
		for j := 0; j < len(objectValue.Refs); j++ {
			objectField := objectValue.Refs[j]
			fieldName := p.definition.ObjectFieldNameBytes(objectField)
			switch {
			case bytes.Equal(fieldName, []byte("name")):
				params[i].name = p.definition.StringValueContentBytes(p.definition.ObjectFieldValue(objectField).Ref)
			case bytes.Equal(fieldName, []byte("sourceKind")):
				params[i].sourceKind = p.definition.EnumValueNameBytes(p.definition.ObjectFieldValue(objectField).Ref)
			case bytes.Equal(fieldName, []byte("sourceName")):
				params[i].sourceName = p.definition.StringValueContentBytes(p.definition.ObjectFieldValue(objectField).Ref)
			case bytes.Equal(fieldName, []byte("variableType")):
				params[i].variableType = p.definition.StringValueContentBytes(p.definition.ObjectFieldValue(objectField).Ref)
			}
		}
	}
	return params
}

type ResolverParameter struct {
	name         []byte
	sourceKind   []byte
	sourceName   []byte
	variableType []byte
}

func (p *planningVisitor) resolverDirectiveFieldName(field int, resolverDefinition ResolverDefinition) []byte {
	definition, exists := p.FieldDefinition(field)
	if !exists {
		return nil
	}

	directive, exists := p.definition.FieldDefinitionDirectiveByName(definition, resolverDefinition.Resolver.DirectiveName())
	if !exists {
		return nil
	}

	value, exists := p.definition.DirectiveArgumentValueByName(directive, []byte("field"))
	if !exists {
		return nil
	}

	if value.Kind != ast.ValueKindString {
		return nil
	}

	return p.definition.StringValueContentBytes(value.Ref)
}

func (p *planningVisitor) quoteValue(valueType int) bool {
	typeName := p.definition.ResolveTypeName(valueType)
	switch {
	case bytes.Equal(typeName, literal.INT):
		return false
	case bytes.Equal(typeName, literal.BOOLEAN):
		return false
	case bytes.Equal(typeName, literal.FLOAT):
		return false
	default:
		return true
	}
}

func (p *planningVisitor) fieldPath(ref int) []string {
	path := []string{
		p.operation.FieldNameString(ref),
	}
	definition, ok := p.FieldDefinition(ref)
	if !ok {
		return path
	}
	directive, ok := p.definition.FieldDefinitionDirectiveByName(definition, []byte("mapTo"))
	if !ok {
		return path
	}
	value, ok := p.definition.DirectiveArgumentValueByName(directive, []byte("objectField"))
	if !ok || value.Kind != ast.ValueKindString {
		return path
	}
	path[0] = p.definition.StringValueContentString(value.Ref)
	return path
}
