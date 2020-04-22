package plan

import (
	"bytes"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astimport"
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
	TypeName          string
	FieldNames        []string
	Attributes        DataSourceAttributes
	DataSourcePlanner DataSourcePlanner
}

type DataSourceAttribute struct {
	Key   string
	Value []byte
}

type DataSourceAttributes []DataSourceAttribute

func (d *DataSourceAttributes) ValueForKey(key string) []byte {
	for i := range *d {
		if (*d)[i].Key == key {
			return (*d)[i].Value
		}
	}
	return nil
}

type Planner struct {
	definition *ast.Document
	visitor    *Visitor
	walker     *astvisitor.Walker
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

	walker.SetVisitorFilter(visitor)
	walker.RegisterDocumentVisitor(visitor)
	walker.RegisterOperationDefinitionVisitor(visitor)
	walker.RegisterSelectionSetVisitor(visitor)
	walker.RegisterFieldVisitor(visitor)

	for i := range config.DataSources {
		config.DataSources[i].DataSourcePlanner.Register(visitor)
	}

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
	DataSources             []DataSourceConfiguration
	Definition, Operation   *ast.Document
	Importer                astimport.Importer
	opName                  []byte
	plan                    Plan
	CurrentObject           *resolve.Object
	currentFields           *[]resolve.Field
	fields                  []*[]resolve.Field
	activeDataSourcePlanner DataSourcePlanner
}

func (v *Visitor) AllowVisitor(visitorKind astvisitor.VisitorKind, ref int, visitor interface{}) bool {
	if visitor == v {
		return true
	}
	switch visitorKind {
	case astvisitor.EnterDocument,
		astvisitor.LeaveDocument:
		return true
	default:
		return visitor == v.activeDataSourcePlanner
	}
}

func (v *Visitor) IsRootField(ref int) (bool, *DataSourceConfiguration) {
	fieldName := v.Operation.FieldNameString(ref)
	enclosingTypeName := v.EnclosingTypeDefinition.Name(v.Definition)
	for i := range v.DataSources {
		if enclosingTypeName != v.DataSources[i].TypeName {
			continue
		}
		for j := range v.DataSources[i].FieldNames {
			if fieldName == v.DataSources[i].FieldNames[j] {
				return true, &v.DataSources[i]
			}
		}
	}
	return false, nil
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

	v.setActiveDataSourcePlanner(fieldNameStr)

	definition, ok := v.Definition.NodeFieldDefinitionByName(v.EnclosingTypeDefinition, fieldName)
	if !ok {
		return
	}
	fieldDefinitionType := v.Definition.FieldDefinitionType(definition)
	typeName := v.Definition.ResolveTypeNameString(fieldDefinitionType)

	var value resolve.Node
	var nextCurrentObject *resolve.Object

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
		nextCurrentObject = obj
	}

	v.Defer(func() {
		if nextCurrentObject != nil {
			v.CurrentObject = nextCurrentObject
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
	})
}

func (v *Visitor) setActiveDataSourcePlanner(currentFieldName string) {
	enclosingTypeName := v.EnclosingTypeDefinition.Name(v.Definition)
	for i := range v.DataSources {
		if v.DataSources[i].TypeName != enclosingTypeName {
			continue
		}
		for j := range v.DataSources[i].FieldNames {
			if v.DataSources[i].FieldNames[j] == currentFieldName {
				v.activeDataSourcePlanner = v.DataSources[i].DataSourcePlanner
				return
			}
		}
	}
}

func (v *Visitor) LeaveField(ref int) {

}
