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

type DataSourceDefinition struct {
	// the type name to which the data source is attached
	TypeName []byte
	// the field on the type to which the data source is attached
	FieldName []byte
	// a factory method to return a new planner
	DataSourcePlannerFactory func() DataSourcePlanner
}

type ResolverDefinitions []DataSourceDefinition

func (r ResolverDefinitions) DefinitionForTypeField(typeName, fieldName []byte, definition *DataSourceDefinition) (exists bool) {
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

func (p *Planner) Plan(operation, definition *ast.Document, report *operationreport.Report) RootNode {
	p.walker.Walk(operation, definition, report)
	return p.visitor.rootNode
}

type planningVisitor struct {
	*astvisitor.Walker
	resolverDefinitions   ResolverDefinitions
	operation, definition *ast.Document
	rootNode              RootNode
	currentNode           []Node
	planners              []dataSourcePlannerRef
}

type dataSourcePlannerRef struct {
	path     ast.Path
	fieldRef int
	planner  DataSourcePlanner
}

func (p *planningVisitor) EnterDocument(operation, definition *ast.Document) {
	p.operation, p.definition = operation, definition
	obj := &Object{}
	p.rootNode = &Object{
		operationType: operation.OperationDefinitions[0].OperationType,
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
	if len(p.planners) != 0 {
		p.planners[len(p.planners)-1].planner.EnterInlineFragment(ref)
	}
}

func (p *planningVisitor) LeaveInlineFragment(ref int) {
	if len(p.planners) != 0 {
		p.planners[len(p.planners)-1].planner.LeaveInlineFragment(ref)
	}
}

func (p *planningVisitor) EnterField(ref int) {

	definition, exists := p.FieldDefinition(ref)
	if !exists {
		return
	}

	resolverTypeName := p.definition.NodeResolverTypeName(p.EnclosingTypeDefinition, p.Path)

	var resolverDefinition DataSourceDefinition
	hasResolverDefinition := p.resolverDefinitions.DefinitionForTypeField(resolverTypeName, p.operation.FieldNameBytes(ref), &resolverDefinition)
	if hasResolverDefinition {

		p.planners = append(p.planners, dataSourcePlannerRef{
			path:     p.Path,
			fieldRef: ref,
			planner:  resolverDefinition.DataSourcePlannerFactory(),
		})

		params := p.resolverDirectiveParamObjectValues(ref, p.planners[len(p.planners)-1].planner)

		var resolveArgs []Argument
		if len(params) != 0 {
			resolveArgs = make([]Argument, 0, len(params))
		}
		for i := 0; i < len(params); i++ {

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
					panic("todo: handle FIELD_ARGUMENTS not exists")
				}
				value := p.operation.ArgumentValue(arg)
				if value.Kind != ast.ValueKindVariable {
					panic("todo: handle value != variable")
				}
				variableName := p.operation.VariableValueNameBytes(value.Ref)
				resolveArgs = append(resolveArgs, &ContextVariableArgument{
					Name:         params[i].sourceName,
					VariableName: variableName,
				})
			}
		}

		p.planners[len(p.planners)-1].planner.Initialize(p.Walker, p.operation, p.definition, resolveArgs, params)
	}

	if len(p.planners) != 0 {
		p.planners[len(p.planners)-1].planner.EnterField(ref)
	}

	switch parent := p.currentNode[len(p.currentNode)-1].(type) {
	case *Object:

		var planner DataSourcePlanner
		resolveRef := p.planners[len(p.planners)-1]
		if resolveRef.path.Equals(p.Path) && resolveRef.fieldRef == ref {
			planner = resolveRef.planner
		}

		path := p.fieldPath(ref)
		if planner != nil {
			path = planner.OverrideRootFieldPath(path)
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

			firstNValue, ok := p.FieldDefinitionDirectiveArgumentValueByName(ref, []byte("ListFilterFirstN"), []byte("n"))
			if ok {
				if firstNValue.Kind == ast.ValueKindInteger {
					firstN := p.definition.IntValueAsInt(firstNValue.Ref)
					list.Filter = &ListFilterFirstN{
						FirstN: firstN,
					}
				}
			}

			parent.Fields = append(parent.Fields, Field{
				Name:  p.operation.FieldNameBytes(ref),
				Value: list,
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
			Name:  p.operation.FieldObjectNameBytes(ref),
			Value: value,
			Skip:  skipCondition,
		})

		p.currentNode = append(p.currentNode, value)
	}
}

func (p *planningVisitor) LeaveField(ref int) {

	var plannedDataSource DataSource
	var plannedArgs []Argument

	if len(p.planners) != 0 {

		p.planners[len(p.planners)-1].planner.LeaveField(ref)

		if p.planners[len(p.planners)-1].path.Equals(p.Path) && p.planners[len(p.planners)-1].fieldRef == ref {
			plannedDataSource, plannedArgs = p.planners[len(p.planners)-1].planner.Plan()
			p.planners = p.planners[:len(p.planners)-1]

			if len(p.currentNode) >= 2 {
				switch parent := p.currentNode[len(p.currentNode)-2].(type) {
				case *Object:
					for i := 0; i < len(parent.Fields); i++ {
						if bytes.Equal(p.operation.FieldObjectNameBytes(ref), parent.Fields[i].Name) {

							pathName := p.operation.FieldObjectNameString(ref)
							parent.Fields[i].HasResolver = true

							singleFetch := &SingleFetch{
								Source: &DataSourceInvocation{
									Args:       plannedArgs,
									DataSource: plannedDataSource,
								},
								BufferName: pathName,
							}

							if parent.Fetch == nil {
								parent.Fetch = singleFetch
							} else {
								switch fetch := parent.Fetch.(type) {
								case *ParallelFetch:
									fetch.Fetches = append(fetch.Fetches, singleFetch)
								case *SerialFetch:
									fetch.Fetches = append(fetch.Fetches, singleFetch)
								case *SingleFetch:
									first := *fetch
									parent.Fetch = &ParallelFetch{
										Fetches: []Fetch{
											&first,
											singleFetch,
										},
									}
								}
							}
						}
					}
				}
			}
		}
	}

	p.currentNode = p.currentNode[:len(p.currentNode)-1]
}

func (p *planningVisitor) EnterSelectionSet(ref int) {
	if len(p.planners) != 0 {
		p.planners[len(p.planners)-1].planner.EnterSelectionSet(ref)
	}
}

func (p *planningVisitor) LeaveSelectionSet(ref int) {
	if len(p.planners) != 0 {
		p.planners[len(p.planners)-1].planner.LeaveSelectionSet(ref)
	}
}

func (p *planningVisitor) resolverDirectiveParamObjectValues(field int, sourcePlanner DataSourcePlanner) []ResolverParameter {
	definition, exists := p.FieldDefinition(field)
	if !exists {
		return nil
	}

	directive, exists := p.definition.FieldDefinitionDirectiveByName(definition, sourcePlanner.DirectiveName())
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
	if ok {
		value, ok := p.definition.DirectiveArgumentValueByName(directive, []byte("objectField"))
		if ok && value.Kind == ast.ValueKindString {
			path[0] = p.definition.StringValueContentString(value.Ref)
		}
	}

	def, ok := p.FieldDefinition(ref)
	if !ok {
		return path
	}
	pathDirective, ok := p.definition.FieldDefinitionDirectiveByName(def, []byte("path"))
	if !ok {
		return path
	}
	appendValue, ok := p.definition.DirectiveArgumentValueByName(pathDirective, []byte("append"))
	if ok && appendValue.Kind == ast.ValueKindList {
		for _, j := range p.definition.ListValues[appendValue.Ref].Refs {
			listValue := p.definition.Values[j]
			if listValue.Kind != ast.ValueKindString {
				continue
			}
			path = append(path, p.definition.StringValueContentString(listValue.Ref))
		}
	}
	prependValue, ok := p.definition.DirectiveArgumentValueByName(pathDirective, []byte("prepend"))
	if ok {
		for _, j := range p.definition.ListValues[prependValue.Ref].Refs {
			listValue := p.definition.Values[j]
			if listValue.Kind != ast.ValueKindString {
				continue
			}
			path = append([]string{p.definition.StringValueContentString(listValue.Ref)}, path...)
		}
	}

	return path
}
