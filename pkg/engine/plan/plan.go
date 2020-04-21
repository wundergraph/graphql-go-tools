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

type DataSourcePlanner interface {
	Register(visitor *Visitor)
}

type DataSourceConfiguration struct {
	TypeName   string
	FieldNames []string
	Attributes []DataSourceAttribute
}

type DataSourceAttribute struct {
	Key   string
	Value interface{}
}

type Planner struct {
	definition *ast.Document
	visitor    *Visitor
	walker     *astvisitor.Walker
}

func (p *Planner) RegisterDataSourcePlanner(planner DataSourcePlanner) {
	planner.Register(p.visitor)
}

type Configuration struct {
	DataSources []DataSourceConfiguration
}

func NewPlanner(definition *ast.Document, config Configuration) *Planner {

	walker := astvisitor.NewWalker(48)
	visitor := &Visitor{
		Walker:      &walker,
		Definition:  definition,
		DataSources: config.DataSources,
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
	p.visitor.Operation = operation
	p.visitor.opName = operationName
	p.walker.Walk(operation, p.definition, report)
	return p.visitor.plan
}

type Visitor struct {
	*astvisitor.Walker
	DataSources           []DataSourceConfiguration
	Definition, Operation *ast.Document
	opName                []byte
	plan                  Plan
	CurrentObject         *resolve.Object
	currentFields         *[]resolve.Field
	fields                []*[]resolve.Field
}

func (v *Visitor) EnterDocument(operation, definition *ast.Document) {

}

func (v *Visitor) LeaveDocument(operation, definition *ast.Document) {

}

func (v *Visitor) EnterOperationDefinition(ref int) {
	if bytes.Equal(v.Operation.OperationDefinitionNameBytes(ref), v.opName) {
		v.CurrentObject = &resolve.Object{}
		v.plan = &SynchronousResponsePlan{
			Response: resolve.GraphQLResponse{
				Data: v.CurrentObject,
			},
		}
	} else {
		v.SkipNode()
	}
}

func (v *Visitor) LeaveOperationDefinition(ref int) {
	v.CurrentObject = nil
}

func (v *Visitor) EnterSelectionSet(ref int) {
	v.CurrentObject.FieldSets = append(v.CurrentObject.FieldSets, resolve.FieldSet{
		Fields: []resolve.Field{},
	})
	v.currentFields = &v.CurrentObject.FieldSets[len(v.CurrentObject.FieldSets)-1].Fields
	v.fields = append(v.fields, v.currentFields)
}

func (v *Visitor) LeaveSelectionSet(ref int) {
	v.fields = v.fields[:len(v.fields)-1]
	if len(v.fields) == 0 {
		return
	}
	v.currentFields = v.fields[len(v.fields)-1]
}

func (v *Visitor) EnterField(ref int) {
	fieldName := v.Operation.FieldNameBytes(ref)
	fieldNameStr := v.Operation.FieldNameString(ref)
	definition, ok := v.Definition.NodeFieldDefinitionByName(v.EnclosingTypeDefinition, fieldName)
	if !ok {
		return
	}
	fieldDefinitionType := v.Definition.FieldDefinitionType(definition)
	typeName := v.Definition.ResolveTypeNameString(fieldDefinitionType)

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
			v.CurrentObject = obj
		}()
	}

	isList := v.Definition.TypeIsList(fieldDefinitionType)
	if isList {
		list := &resolve.Array{
			Path: []string{fieldNameStr},
			Item: value,
		}
		value = list
	}

	if v.Operation.FieldAliasIsDefined(ref) {
		fieldName = v.Operation.FieldAliasBytes(ref)
	}

	*v.currentFields = append(*v.currentFields, resolve.Field{
		Name:  fieldName,
		Value: value,
	})
}

func (v *Visitor) LeaveField(ref int) {

}
