package plan

import (
	"bytes"
	"regexp"
	"strings"

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

type FieldConfiguration struct {
	TypeName          string
	FieldNames        []string
	Attributes        DataSourceAttributes
	DataSourcePlanner DataSourcePlanner
	FieldMappings     []FieldMapping
}

type FieldMapping struct {
	FieldName             string
	DisableDefaultMapping bool
	Mapping               []string
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
	FieldConfigurations []FieldConfiguration
}

func NewPlanner(definition *ast.Document, config Configuration) *Planner {

	walker := astvisitor.NewWalker(48)
	visitor := &Visitor{
		Walker:              &walker,
		Definition:          definition,
		FieldConfigurations: config.FieldConfigurations,
	}

	walker.SetVisitorFilter(visitor)
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterOperationDefinitionVisitor(visitor)
	walker.RegisterSelectionSetVisitor(visitor)
	walker.RegisterEnterFieldVisitor(visitor)

	for i := range config.FieldConfigurations {
		if config.FieldConfigurations[i].DataSourcePlanner == nil {
			continue
		}
		config.FieldConfigurations[i].DataSourcePlanner.Register(visitor)
	}

	walker.RegisterLeaveFieldVisitor(visitor)
	walker.RegisterLeaveDocumentVisitor(visitor)

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
	FieldConfigurations     []FieldConfiguration
	Definition, Operation   *ast.Document
	Importer                astimport.Importer
	opName                  []byte
	plan                    Plan
	currentObject           *resolve.Object
	objects                 []fieldObject
	currentFields           *[]resolve.Field
	fields                  []*[]resolve.Field
	fetches                 resolve.Fetches
	activeDataSourcePlanner DataSourcePlanner
}

func (v *Visitor) SetCurrentObjectFetch(fetch resolve.Fetch) {
	v.currentObject.Fetch = fetch
	v.fetches = append(v.fetches, fetch)
}

func (v *Visitor) CurrentObjectHasFetch() bool {
	return v.currentObject.Fetch != nil
}

type fieldObject struct {
	fieldRef int
	object   *resolve.Object
}

func (v *Visitor) NextBufferID() uint8 {
	return 0
}

func (v *Visitor) SetBufferIDForCurrentFieldSet(bufferID uint8) {
	if v.currentObject == nil {
		return
	}
	if len(v.currentObject.FieldSets) == 0 {
		return
	}
	v.currentObject.FieldSets[len(v.currentObject.FieldSets)-1].HasBuffer = true
	v.currentObject.FieldSets[len(v.currentObject.FieldSets)-1].BufferID = bufferID
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

func (v *Visitor) IsRootField(ref int) (bool, *FieldConfiguration) {
	fieldName := v.Operation.FieldNameString(ref)
	enclosingTypeName := v.EnclosingTypeDefinition.Name(v.Definition)
	for i := range v.FieldConfigurations {
		if enclosingTypeName != v.FieldConfigurations[i].TypeName {
			continue
		}
		for j := range v.FieldConfigurations[i].FieldNames {
			if fieldName == v.FieldConfigurations[i].FieldNames[j] {
				return true, &v.FieldConfigurations[i]
			}
		}
	}
	return false, nil
}

func (v *Visitor) EnterDocument(operation, definition *ast.Document) {
	v.fields = v.fields[:0]
	v.objects = v.objects[:0]
	v.fetches = v.fetches[:0]
}

func (v *Visitor) LeaveDocument(operation, definition *ast.Document) {
	for i := range v.fetches {
		v.prepareFetchVariables(v.fetches[i])
	}
}

var (
	templateRegex = regexp.MustCompile(`{{.*?}}`)
	selectorRegex = regexp.MustCompile(`{{\s(.*?)\s}}`)
)

func (v *Visitor) prepareFetchVariables(fetch resolve.Fetch) {
	switch f := fetch.(type) {
	case *resolve.SingleFetch:
		f.Input = templateRegex.ReplaceAllFunc(f.Input, func(i []byte) []byte {
			selector := selectorRegex.FindSubmatch(i)
			if len(selector) != 2 {
				return i
			}
			path := strings.TrimPrefix(string(selector[1]), ".")
			segments := strings.Split(path, ".")
			if len(segments) < 2 {
				return i
			}
			switch segments[0] {
			case "object":
				return f.Variables.AddVariable(&resolve.ObjectVariable{Path: segments[1:]})
			case "arguments":
				return f.Variables.AddVariable(&resolve.ContextVariable{Path: segments[1:]})
			default:
				return i
			}
		})
	}
}

func (v *Visitor) EnterOperationDefinition(ref int) {
	if bytes.Equal(v.Operation.OperationDefinitionNameBytes(ref), v.opName) {
		v.currentObject = &resolve.Object{}
		v.objects = append(v.objects, fieldObject{
			object:   v.currentObject,
			fieldRef: -1,
		})
		v.plan = &SynchronousResponsePlan{
			Response: resolve.GraphQLResponse{
				Data: v.currentObject,
			},
		}
	} else {
		v.SkipNode()
	}
}

func (v *Visitor) LeaveOperationDefinition(ref int) {
	v.currentObject = nil
}

func (v *Visitor) EnterSelectionSet(ref int) {
	v.currentObject.FieldSets = append(v.currentObject.FieldSets, resolve.FieldSet{
		Fields: []resolve.Field{},
	})
	v.currentFields = &v.currentObject.FieldSets[len(v.currentObject.FieldSets)-1].Fields
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

	isList := v.Definition.TypeIsList(fieldDefinitionType)
	isRootField, _ := v.IsRootField(ref)

	var value resolve.Node
	var nextCurrentObject *resolve.Object

	path := v.resolveFieldPath(ref)

	switch typeName {
	case "String":
		value = &resolve.String{
			Path: path,
		}
	case "Boolean":
		value = &resolve.Boolean{
			Path: path,
		}
	case "Int":
		value = &resolve.Integer{
			Path: path,
		}
	case "Float":
		value = &resolve.Float{
			Path: path,
		}
	default:
		obj := &resolve.Object{}
		if !isRootField && !isList {
			obj.Path = path
		}
		value = obj
		nextCurrentObject = obj
	}

	v.Defer(func() {
		if nextCurrentObject != nil {
			v.currentObject = nextCurrentObject
			v.objects = append(v.objects, fieldObject{
				fieldRef: ref,
				object:   nextCurrentObject,
			})
		}
		if isList {
			list := &resolve.Array{
				Path: path,
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

func (v *Visitor) resolveFieldPath(ref int) []string {
	typeName := v.EnclosingTypeDefinition.Name(v.Definition)
	fieldName := v.Operation.FieldNameString(ref)
	for i := range v.FieldConfigurations {
		if v.FieldConfigurations[i].TypeName == typeName {
			for j := range v.FieldConfigurations[i].FieldMappings {
				if v.FieldConfigurations[i].FieldMappings[j].FieldName == fieldName {
					if v.FieldConfigurations[i].FieldMappings[j].Mapping != nil {
						return v.FieldConfigurations[i].FieldMappings[j].Mapping
					}
					if v.FieldConfigurations[i].FieldMappings[j].DisableDefaultMapping {
						return nil
					}
					return []string{fieldName}
				}
			}
		}
	}
	return []string{fieldName}
}

func (v *Visitor) setActiveDataSourcePlanner(currentFieldName string) {
	enclosingTypeName := v.EnclosingTypeDefinition.Name(v.Definition)
	for i := range v.FieldConfigurations {
		if v.FieldConfigurations[i].TypeName != enclosingTypeName {
			continue
		}
		for j := range v.FieldConfigurations[i].FieldNames {
			if v.FieldConfigurations[i].FieldNames[j] == currentFieldName {
				v.activeDataSourcePlanner = v.FieldConfigurations[i].DataSourcePlanner
				return
			}
		}
	}
}

func (v *Visitor) LeaveField(ref int) {

	if len(v.objects) < 2 {
		return
	}
	if v.objects[len(v.objects)-1].fieldRef == ref {
		v.objects = v.objects[:len(v.objects)-1]
		v.currentObject = v.objects[len(v.objects)-1].object
	}
}
