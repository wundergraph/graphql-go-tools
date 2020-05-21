package plan

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/buger/jsonparser"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astimport"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type Kind int
type enterOrLeave int

const (
	SynchronousResponseKind Kind = iota + 1
	StreamingResponseKind
	SubscriptionResponseKind

	enter enterOrLeave = iota + 1
	leave
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

type FieldMapping struct {
	TypeName              string
	FieldName             string
	DisableDefaultMapping bool
	Path                  []string
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
	DataSourceConfigurations []DataSourceConfiguration
	FieldMappings            []FieldMapping
}

func NewPlanner(definition *ast.Document, config Configuration) *Planner {

	walker := astvisitor.NewWalker(48)
	visitor := &Visitor{
		Walker:                   &walker,
		Definition:               definition,
		DataSourceConfigurations: config.DataSourceConfigurations,
		FieldMappings:            config.FieldMappings,
	}

	walker.SetVisitorFilter(visitor)
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterOperationDefinitionVisitor(visitor)
	walker.RegisterSelectionSetVisitor(visitor)
	walker.RegisterEnterFieldVisitor(visitor)
	walker.RegisterEnterArgumentVisitor(visitor)

	for i := range config.DataSourceConfigurations {
		if config.DataSourceConfigurations[i].DataSourcePlanner == nil {
			continue
		}
		config.DataSourceConfigurations[i].DataSourcePlanner.Register(visitor)
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
	DataSourceConfigurations            []DataSourceConfiguration
	FieldMappings                       []FieldMapping
	Definition, Operation               *ast.Document
	Importer                            astimport.Importer
	opName                              []byte
	plan                                Plan
	currentObject                       *resolve.Object
	objects                             []fieldObject
	currentFields                       *[]resolve.Field
	fields                              []*[]resolve.Field
	fetchConfigs                        []fetchConfig
	activeDataSourcePlanner             DataSourcePlanner
	fieldArguments                      []fieldArgument
	nextBufferID                        int
	popFieldsOnLeaveField               []int
	fieldDataSourcePlanners             []fieldDataSourcePlanner
	currentFieldName                    string
	currentFieldEnclosingTypeDefinition ast.Node
}

type fieldDataSourcePlanner struct {
	field   int
	planner DataSourcePlanner
}

type fetchConfig struct {
	fetch              resolve.Fetch
	fieldConfiguration *DataSourceConfiguration
}

func (v *Visitor) SetCurrentObjectFetch(fetch *resolve.SingleFetch, config *DataSourceConfiguration) {
	if v.currentObject.Fetch != nil {
		switch current := v.currentObject.Fetch.(type) {
		case *resolve.SingleFetch:
			parallel := &resolve.ParallelFetch{
				Fetches: []*resolve.SingleFetch{
					current,
					fetch,
				},
			}
			v.currentObject.Fetch = parallel
		case *resolve.ParallelFetch:
			current.Fetches = append(current.Fetches, fetch)
		}
		return
	}
	v.currentObject.Fetch = fetch
	v.fetchConfigs = append(v.fetchConfigs, fetchConfig{fetch: fetch, fieldConfiguration: config})
}

type fieldObject struct {
	fieldRef int
	object   *resolve.Object
}

func (v *Visitor) NextBufferID() int {
	v.nextBufferID++
	return v.nextBufferID
}

func (v *Visitor) SetBufferIDForCurrentFieldSet(bufferID int) {
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

func (v *Visitor) IsRootField(ref int) (bool, *DataSourceConfiguration) {
	fieldName := v.Operation.FieldNameString(ref)
	enclosingTypeName := v.EnclosingTypeDefinition.Name(v.Definition)
	for i := range v.DataSourceConfigurations {
		if enclosingTypeName != v.DataSourceConfigurations[i].TypeName {
			continue
		}
		for j := range v.DataSourceConfigurations[i].FieldNames {
			if fieldName == v.DataSourceConfigurations[i].FieldNames[j] {
				return true, &v.DataSourceConfigurations[i]
			}
		}
	}
	return false, nil
}

func (v *Visitor) EnterDocument(operation, definition *ast.Document) {
	v.fields = v.fields[:0]
	v.objects = v.objects[:0]
	v.fetchConfigs = v.fetchConfigs[:0]
	v.fieldArguments = v.fieldArguments[:0]
	v.popFieldsOnLeaveField = v.popFieldsOnLeaveField[:0]
	v.fieldDataSourcePlanners = v.fieldDataSourcePlanners[:0]
	v.nextBufferID = -1
}

func (v *Visitor) LeaveDocument(operation, definition *ast.Document) {
	for i := range v.fetchConfigs {
		v.prepareFetchVariables(v.fetchConfigs[i])
	}
}

var (
	templateRegex = regexp.MustCompile(`{{.*?}}`)
	selectorRegex = regexp.MustCompile(`{{\s(.*?)\s}}`)
)

func (v *Visitor) prepareFetchVariables(config fetchConfig) {
	switch f := config.fetch.(type) {
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
				variableName, _ := f.Variables.AddVariable(&resolve.ObjectVariable{Path: segments[1:]})
				return variableName
			case "arguments":
				segments = segments[1:]
				if len(segments) < 2 {
					return i
				}
				for j := range v.fieldArguments {
					if v.fieldArguments[j].typeName == config.fieldConfiguration.TypeName &&
						v.fieldArguments[j].fieldName == segments[0] &&
						v.fieldArguments[j].argumentName == segments[1] {
						segments = segments[2:]
						switch v.fieldArguments[j].kind {
						case fieldArgumentTypeVariable:
							variablePath := append([]string{string(v.fieldArguments[j].value)}, segments...)
							variableName, _ := f.Variables.AddVariable(&resolve.ContextVariable{Path: variablePath})
							return variableName
						case fieldArgumentTypeStatic:
							if len(segments) == 0 {
								return v.fieldArguments[j].value
							}
							i, _, _, _ = jsonparser.Get(v.fieldArguments[j].value, segments...)
							return i
						}
					}
				}
				return i
			default:
				return i
			}
		})
	}
}

func (v *Visitor) EnterArgument(ref int) {
	if v.Ancestors[len(v.Ancestors)-1].Kind != ast.NodeKindField {
		return
	}
	value := v.Operation.ArgumentValue(ref)
	arg := fieldArgument{
		typeName:     v.currentFieldEnclosingTypeDefinition.Name(v.Definition),
		fieldName:    v.currentFieldName,
		argumentName: v.Operation.ArgumentNameString(ref),
		kind:         fieldArgumentTypeStatic,
	}
	switch value.Kind {
	case ast.ValueKindVariable:
		arg.kind = fieldArgumentTypeVariable
		value := v.Operation.VariableValueNameBytes(value.Ref)
		arg.value = make([]byte, len(value))
		copy(arg.value, value)
	case ast.ValueKindString:
		value := v.Operation.StringValueContentBytes(value.Ref)
		arg.value = make([]byte, len(value))
		copy(arg.value, value)
	case ast.ValueKindBoolean:
		switch v.Operation.BooleanValues[value.Ref] {
		case true:
			arg.value = literal.TRUE
		case false:
			arg.value = literal.FALSE
		default:
			return
		}
	case ast.ValueKindFloat:
		value := v.Operation.FloatValueRaw(value.Ref)
		arg.value = make([]byte, len(value))
		copy(arg.value, value)
	case ast.ValueKindInteger:
		value := v.Operation.IntValueRaw(value.Ref)
		arg.value = make([]byte, len(value))
		copy(arg.value, value)
	case ast.ValueKindEnum:
		value := v.Operation.EnumValueNameBytes(value.Ref)
		arg.value = make([]byte, len(value))
		copy(arg.value, value)
	case ast.ValueKindNull:
		arg.value = literal.NULL
	default:
		return
	}
	v.fieldArguments = append(v.fieldArguments, arg)
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

	if isRoot, _ := v.IsRootField(ref); isRoot {
		if len(*v.currentFields) != 0 {
			v.currentObject.FieldSets = append(v.currentObject.FieldSets, resolve.FieldSet{
				Fields: []resolve.Field{},
			})
			v.currentFields = &v.currentObject.FieldSets[len(v.currentObject.FieldSets)-1].Fields
			v.fields = append(v.fields, v.currentFields)
			v.popFieldsOnLeaveField = append(v.popFieldsOnLeaveField, ref)
		}
	}

	fieldName := v.Operation.FieldNameBytes(ref)
	fieldNameStr := v.Operation.FieldNameString(ref)

	v.currentFieldName = fieldNameStr
	v.currentFieldEnclosingTypeDefinition = v.EnclosingTypeDefinition

	v.setActiveDataSourcePlanner(ref, enter)

	definition, ok := v.Definition.NodeFieldDefinitionByName(v.EnclosingTypeDefinition, fieldName)
	if !ok {
		return
	}
	fieldDefinitionType := v.Definition.FieldDefinitionType(definition)
	typeName := v.Definition.ResolveTypeNameString(fieldDefinitionType)

	isList := v.Definition.TypeIsList(fieldDefinitionType)

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
		if !isList {
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
			alias := v.Operation.FieldAliasBytes(ref)
			fieldName = make([]byte, len(alias))
			copy(fieldName, alias)
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
	for i := range v.FieldMappings {
		if v.FieldMappings[i].TypeName == typeName && v.FieldMappings[i].FieldName == fieldName {
			if v.FieldMappings[i].Path != nil {
				return v.FieldMappings[i].Path
			}
			if v.FieldMappings[i].DisableDefaultMapping {
				return nil
			}
			return []string{fieldName}
		}
	}
	return []string{fieldName}
}

func (v *Visitor) setActiveDataSourcePlanner(fieldRef int, enterOrLeave enterOrLeave) {

	if enterOrLeave == leave {
		if len(v.fieldDataSourcePlanners) == 0 {
			return
		}
		if v.fieldDataSourcePlanners[len(v.fieldDataSourcePlanners)-1].field == fieldRef {
			v.fieldDataSourcePlanners = v.fieldDataSourcePlanners[:len(v.fieldDataSourcePlanners)-1]
			if len(v.fieldDataSourcePlanners) != 0 {
				v.activeDataSourcePlanner = v.fieldDataSourcePlanners[len(v.fieldDataSourcePlanners)-1].planner
			}
		}
		return
	}

	fieldName := v.Operation.FieldNameString(fieldRef)

	enclosingTypeName := v.EnclosingTypeDefinition.Name(v.Definition)
	for i := range v.DataSourceConfigurations {
		if v.DataSourceConfigurations[i].TypeName != enclosingTypeName {
			continue
		}
		for j := range v.DataSourceConfigurations[i].FieldNames {
			if v.DataSourceConfigurations[i].FieldNames[j] == fieldName {
				v.activeDataSourcePlanner = v.DataSourceConfigurations[i].DataSourcePlanner
				v.fieldDataSourcePlanners = append(v.fieldDataSourcePlanners, fieldDataSourcePlanner{
					field:   fieldRef,
					planner: v.DataSourceConfigurations[i].DataSourcePlanner,
				})
				return
			}
		}
	}
}

func (v *Visitor) LeaveField(ref int) {

	if len(v.popFieldsOnLeaveField) != 0 && v.popFieldsOnLeaveField[len(v.popFieldsOnLeaveField)-1] == ref {
		v.popFieldsOnLeaveField = v.popFieldsOnLeaveField[:len(v.popFieldsOnLeaveField)-1]
		v.fields = v.fields[:len(v.fields)-1]
		if len(v.fields) != 0 {
			v.currentFields = v.fields[len(v.fields)-1]
		}
	}

	if len(v.objects) < 2 {
		return
	}
	if v.objects[len(v.objects)-1].fieldRef == ref {
		v.objects = v.objects[:len(v.objects)-1]
		v.currentObject = v.objects[len(v.objects)-1].object
	}

	v.setActiveDataSourcePlanner(ref, leave)
}

type fieldArgument struct {
	typeName     string
	fieldName    string
	argumentName string
	kind         fieldArgumentType
	value        []byte
}

type fieldArgumentType int

const (
	fieldArgumentTypeStatic fieldArgumentType = iota + 1
	fieldArgumentTypeVariable
)
