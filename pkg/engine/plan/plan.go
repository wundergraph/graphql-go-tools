package plan

import (
	"bytes"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type Kind int

const (
	SynchronousResponseKind Kind = iota + 1
	StreamingResponseKind
	SubscriptionResponseKind
)

type Reference struct {
	Id   int
	Kind Kind
}

type Plan interface {
	PlanKind() Kind
}

type SynchronousResponsePlan struct {
	Response resolve.GraphQLResponse
}

func (_ *SynchronousResponsePlan) PlanKind() Kind {
	return SynchronousResponseKind
}

type StreamingResponsePlan struct {
}

func (_ *StreamingResponsePlan) PlanKind() Kind {
	return StreamingResponseKind
}

type SubscriptionResponsePlan struct {
}

func (_ *SubscriptionResponsePlan) PlanKind() Kind {
	return SubscriptionResponseKind
}

type Planner struct {
	definition *ast.Document
	visitor    *visitor
	walker     *astvisitor.Walker
}

func NewPlanner(definition *ast.Document) *Planner {

	walker := astvisitor.NewWalker(48)
	visitor := &visitor{
		Walker:     &walker,
		definition: definition,
	}

	walker.RegisterDocumentVisitor(visitor)
	walker.RegisterOperationDefinitionVisitor(visitor)
	walker.RegisterSelectionSetVisitor(visitor)
	walker.RegisterFieldVisitor(visitor)

	return &Planner{
		definition: definition,
		visitor:    visitor,
		walker:     &walker,
	}
}

func (p *Planner) Plan(operation *ast.Document, operationName []byte, report *operationreport.Report) (plan Plan) {
	p.visitor.operation = operation
	p.visitor.opName = operationName
	p.walker.Walk(operation, p.definition, report)
	return p.visitor.plan
}

type visitor struct {
	*astvisitor.Walker
	definition, operation *ast.Document
	opName                []byte
	plan                  Plan
	currentObject         *resolve.Object
	currentFields         *[]resolve.Field
	fields                []*[]resolve.Field
}

func (v *visitor) EnterDocument(operation, definition *ast.Document) {

}

func (v *visitor) LeaveDocument(operation, definition *ast.Document) {

}

func (v *visitor) EnterOperationDefinition(ref int) {
	if bytes.Equal(v.operation.OperationDefinitionNameBytes(ref), v.opName) {
		v.currentObject = &resolve.Object{}
		v.plan = &SynchronousResponsePlan{
			Response: resolve.GraphQLResponse{
				Data: v.currentObject,
			},
		}
	} else {
		v.SkipNode()
	}
}

func (v *visitor) LeaveOperationDefinition(ref int) {
	v.currentObject = nil
}

func (v *visitor) EnterSelectionSet(ref int) {
	v.currentObject.FieldSets = append(v.currentObject.FieldSets, resolve.FieldSet{
		Fields: []resolve.Field{},
	})
	v.currentFields = &v.currentObject.FieldSets[len(v.currentObject.FieldSets)-1].Fields
	v.fields = append(v.fields, v.currentFields)
}

func (v *visitor) LeaveSelectionSet(ref int) {
	v.fields = v.fields[:len(v.fields)-1]
	if len(v.fields) == 0 {
		return
	}
	v.currentFields = v.fields[len(v.fields)-1]
}

func (v *visitor) EnterField(ref int) {
	fieldName := v.operation.FieldNameBytes(ref)
	fieldNameStr := v.operation.FieldNameString(ref)
	definition, ok := v.definition.NodeFieldDefinitionByName(v.EnclosingTypeDefinition, fieldName)
	if !ok {
		return
	}
	fieldDefinitionType := v.definition.FieldDefinitionType(definition)
	typeName := v.definition.ResolveTypeNameString(fieldDefinitionType)

	var value resolve.Node

	switch typeName {
	case "String":
		value = &resolve.String{
			Path: []string{fieldNameStr},
		}
	case "Boolean":
		value = &resolve.Boolean{
			Path: []string{fieldNameStr},
		}
	case "Int":
		value = &resolve.Integer{
			Path: []string{fieldNameStr},
		}
	case "Float":
		value = &resolve.Float{
			Path: []string{fieldNameStr},
		}
	default:
		obj := &resolve.Object{}
		value = obj
		defer func() {
			v.currentObject = obj
		}()
	}

	isList := v.definition.TypeIsList(fieldDefinitionType)
	if isList {
		list := &resolve.Array{
			Path: []string{fieldNameStr},
			Item: value,
		}
		value = list
	}

	if v.operation.FieldAliasIsDefined(ref){
		fieldName = v.operation.FieldAliasBytes(ref)
	}

	*v.currentFields = append(*v.currentFields, resolve.Field{
		Name:  fieldName,
		Value: value,
	})
}

func (v *visitor) LeaveField(ref int) {

}
