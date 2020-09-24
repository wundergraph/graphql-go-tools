package plan

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash"

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
	Response resolve.GraphQLStreamingResponse
}

func (_ *StreamingResponsePlan) PlanKind() Kind {
	return StreamingResponseKind
}

type SubscriptionResponsePlan struct {
	Response resolve.GraphQLSubscription
}

func (_ *SubscriptionResponsePlan) PlanKind() Kind {
	return SubscriptionResponseKind
}

type DataSourcePlanner interface {
	Register(visitor *Visitor)
}

type DataSourceConfiguration struct {
	TypeName                 string
	FieldNames               []string
	Attributes               DataSourceAttributes
	DataSourcePlanner        DataSourcePlanner
	UpstreamUniqueIdentifier string
}

type FieldMapping struct {
	TypeName              string
	FieldName             string
	DisableDefaultMapping bool
	Path                  []string
}

type DataSourceAttribute struct {
	Key   string
	Value json.RawMessage
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
	FieldDependencies        []FieldDependency
}

type FieldDependency struct {
	TypeName       string
	FieldName      string
	RequiresFields []string
}

func NewPlanner(definition *ast.Document, config Configuration) *Planner {

	walker := astvisitor.NewWalker(48)
	visitor := &Visitor{
		Walker:     &walker,
		Definition: definition,
		Config:     config,
	}

	walker.SetVisitorFilter(visitor)
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterOperationDefinitionVisitor(visitor)
	walker.RegisterSelectionSetVisitor(visitor)
	walker.RegisterEnterFieldVisitor(visitor)
	walker.RegisterEnterArgumentVisitor(visitor)
	walker.RegisterEnterDirectiveVisitor(visitor)

	registered := make([]DataSourcePlanner, 0, len(config.DataSourceConfigurations))

Next:
	for i := range config.DataSourceConfigurations {
		if config.DataSourceConfigurations[i].DataSourcePlanner == nil {
			continue
		}
		for j := range registered {
			if registered[j] == config.DataSourceConfigurations[i].DataSourcePlanner {
				continue Next
			}
		}
		config.DataSourceConfigurations[i].DataSourcePlanner.Register(visitor)
		registered = append(registered, config.DataSourceConfigurations[i].DataSourcePlanner)
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
	Config                              Configuration
	Definition, Operation               *ast.Document
	Importer                            astimport.Importer
	opName                              []byte
	plan                                Plan
	currentObject                       *resolve.Object
	currentArray                        *resolve.Array
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
	fieldPathOverrides                  map[int]PathOverrideFunc
	subscription                        *resolve.GraphQLSubscription
	subscriptionDataSourceConfiguration *DataSourceConfiguration
}

type PathOverrideFunc func(path []string) []string

type fieldDataSourcePlanner struct {
	field              int
	planner            DataSourcePlanner
	upstreamIdentifier string
}

type fetchConfig struct {
	fetch              resolve.Fetch
	fieldConfiguration *DataSourceConfiguration
}

// SetFieldPathOverride delegates to a data source planner the possibility to override the path selector for a field.
// E.g. the GraphQL data source allows to define a field alias in the upstream query.
// This means that a data source is able to override the shape of the graphql response.
// In order for the planner to correctly set JSON path selectors for these fields it needs to be able to delegate
// to the data source to override the JSON path of said field.
//
// In short, when a data source overrides the JSON response shape it must also override the JSON selectors
// by setting an override for each field.
func (v *Visitor) SetFieldPathOverride(field int, override PathOverrideFunc) {
	v.fieldPathOverrides[field] = override
}

func (v *Visitor) SetSubscriptionTrigger(trigger resolve.GraphQLSubscriptionTrigger, config DataSourceConfiguration) {
	v.subscription.Trigger = trigger
	v.subscriptionDataSourceConfiguration = &config
}

func (v *Visitor) SetCurrentObjectFetch(fetch *resolve.SingleFetch, config *DataSourceConfiguration) {
	v.fetchConfigs = append(v.fetchConfigs, fetchConfig{fetch: fetch, fieldConfiguration: config})
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
	for i := range v.Config.DataSourceConfigurations {
		if enclosingTypeName != v.Config.DataSourceConfigurations[i].TypeName {
			continue
		}
		fieldMatches := false
		for m := range v.Config.DataSourceConfigurations[i].FieldNames {
			if v.Config.DataSourceConfigurations[i].FieldNames[m] == fieldName {
				fieldMatches = true
				break
			}
		}
		if !fieldMatches {
			continue
		}
		for k := range v.fieldDataSourcePlanners {
			if v.fieldDataSourcePlanners[k].planner == v.Config.DataSourceConfigurations[i].DataSourcePlanner &&
				v.fieldDataSourcePlanners[k].upstreamIdentifier == v.Config.DataSourceConfigurations[i].UpstreamUniqueIdentifier {
				for l := range v.Ancestors {
					if v.Ancestors[l].Kind == ast.NodeKindField &&
						v.Ancestors[l].Ref == v.fieldDataSourcePlanners[k].field {
						return false, &v.Config.DataSourceConfigurations[i]
					}
				}
			}
		}
		for j := range v.Config.DataSourceConfigurations[i].FieldNames {
			if fieldName == v.Config.DataSourceConfigurations[i].FieldNames[j] {
				return true, &v.Config.DataSourceConfigurations[i]
			}
		}
	}
	return false, nil
}

func (v *Visitor) EnterDocument(_, _ *ast.Document) {
	v.fields = v.fields[:0]
	v.objects = v.objects[:0]
	v.fetchConfigs = v.fetchConfigs[:0]
	v.fieldArguments = v.fieldArguments[:0]
	v.popFieldsOnLeaveField = v.popFieldsOnLeaveField[:0]
	v.fieldDataSourcePlanners = v.fieldDataSourcePlanners[:0]
	v.nextBufferID = -1
	v.activeDataSourcePlanner = nil
	v.subscription = nil
	v.subscriptionDataSourceConfiguration = nil
	if v.fieldPathOverrides == nil {
		v.fieldPathOverrides = make(map[int]PathOverrideFunc, 8)
	} else {
		for key := range v.fieldPathOverrides {
			delete(v.fieldPathOverrides, key)
		}
	}
}

func (v *Visitor) LeaveDocument(_, _ *ast.Document) {
	for i := range v.fetchConfigs {
		switch f := v.fetchConfigs[i].fetch.(type) {
		case *resolve.SingleFetch:
			v.prepareSingleFetchVariables(&f.Input, &f.InputTemplate, &f.Variables, v.fetchConfigs[i].fieldConfiguration)
		case *resolve.ParallelFetch:
			for j := range f.Fetches {
				v.prepareSingleFetchVariables(&f.Fetches[j].Input, &f.Fetches[j].InputTemplate, &f.Fetches[j].Variables, v.fetchConfigs[i].fieldConfiguration)
			}
		}
	}
	if v.subscription == nil || v.subscriptionDataSourceConfiguration == nil {
		return
	}
	v.prepareSingleFetchVariables(&v.subscription.Trigger.Input, &v.subscription.Trigger.InputTemplate, &v.subscription.Trigger.Variables, v.subscriptionDataSourceConfiguration)
}

func (v *Visitor) EnterDirective(ref int) {
	directiveName := v.Operation.DirectiveNameString(ref)
	switch directiveName {
	case "defer":
		if v.currentFields == nil || len(*v.currentFields) == 0 {
			return
		}
		(*v.currentFields)[len(*v.currentFields)-1].Defer = true
	case "stream":
		if v.currentArray == nil {
			return
		}
		v.currentArray.Stream.Enabled = true
		v.currentArray.Stream.InitialBatchSize = 0
		value,ok := v.Operation.DirectiveArgumentValueByName(ref,literal.INITIAL_BATCH_SIZE)
		if !ok {
			return
		}
		if value.Kind != ast.ValueKindInteger {
			return
		}
		v.currentArray.Stream.InitialBatchSize = int(v.Operation.IntValueAsInt(value.Ref))
	}
}

var (
	templateRegex = regexp.MustCompile(`{{.*?}}`)
	selectorRegex = regexp.MustCompile(`{{\s(.*?)\s}}`)
)

func (v *Visitor) prepareSingleFetchVariables(input *string, inputTemplate *resolve.InputTemplate, variables *resolve.Variables, config *DataSourceConfiguration) {
	*input = templateRegex.ReplaceAllStringFunc(*input, func(i string) string {
		selector := selectorRegex.FindStringSubmatch(i)
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
			variableName, _ := variables.AddVariable(&resolve.ObjectVariable{Path: segments[1:]}, false)
			return variableName
		case "arguments":
			segments = segments[1:]
			if len(segments) < 1 {
				return i
			}
			for j := range v.fieldArguments {
				if v.fieldArguments[j].typeName == config.TypeName &&
					v.fieldArguments[j].argumentName == segments[0] {
					segments = segments[1:]
					switch v.fieldArguments[j].kind {
					case fieldArgumentTypeVariable:
						variablePath := append([]string{string(v.fieldArguments[j].value)}, segments...)
						variableName, _ := variables.AddVariable(&resolve.ContextVariable{Path: variablePath}, false)
						return variableName
					case fieldArgumentTypeStatic:
						if len(segments) == 0 {
							return string(v.fieldArguments[j].value)
						}
						i, _ = jsonparser.GetString(v.fieldArguments[j].value, segments...)
						return i
					}
				}
			}
			return i
		default:
			return i
		}
	})

	segments := strings.Split(*input, "$$")
	isVariable := false
	for _, seg := range segments {
		switch {
		case isVariable:
			i, _ := strconv.Atoi(seg)
			switch v := (*variables)[i].(type) {
			case *resolve.ContextVariable:
				inputTemplate.Segments = append(inputTemplate.Segments, resolve.TemplateSegment{
					SegmentType:        resolve.VariableSegmentType,
					VariableSource:     resolve.VariableSourceContext,
					VariableSourcePath: v.Path,
				})
			case *resolve.ObjectVariable:
				inputTemplate.Segments = append(inputTemplate.Segments, resolve.TemplateSegment{
					SegmentType:        resolve.VariableSegmentType,
					VariableSource:     resolve.VariableSourceObject,
					VariableSourcePath: v.Path,
				})
			}
			isVariable = false
		default:
			inputTemplate.Segments = append(inputTemplate.Segments, resolve.TemplateSegment{
				SegmentType: resolve.StaticSegmentType,
				Data:        []byte(seg),
			})
			isVariable = true
		}
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
		switch v.Operation.OperationDefinitions[ref].OperationType {
		case ast.OperationTypeQuery, ast.OperationTypeMutation:
			v.plan = &SynchronousResponsePlan{
				Response: resolve.GraphQLResponse{
					Data: v.currentObject,
				},
			}
		case ast.OperationTypeSubscription:
			plan := &SubscriptionResponsePlan{
				Response: resolve.GraphQLSubscription{
					Response: &resolve.GraphQLResponse{
						Data: v.currentObject,
					},
				},
			}
			v.plan = plan
			v.subscription = &plan.Response
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
	typeNameBytes := v.Definition.ResolveTypeNameBytes(fieldDefinitionType)

	isList := v.Definition.TypeIsList(fieldDefinitionType)
	fieldTypeIsNullable := !v.Definition.TypeIsNonNull(fieldDefinitionType)

	var value resolve.Node
	var nextCurrentObject *resolve.Object

	path := v.resolveFieldPath(ref)

	switch string(typeNameBytes) {
	case "String":
		str := &resolve.String{
			Nullable: fieldTypeIsNullable,
		}
		if !isList {
			str.Path = path
			v.Defer(func() {
				if override, ok := v.fieldPathOverrides[ref]; ok {
					str.Path = override(str.Path)
				}
			})
		}
		value = str
	case "Boolean":
		boolean := &resolve.Boolean{
			Nullable: fieldTypeIsNullable,
		}
		if !isList {
			boolean.Path = path
			v.Defer(func() {
				if override, ok := v.fieldPathOverrides[ref]; ok {
					boolean.Path = override(boolean.Path)
				}
			})
		}
		value = boolean
	case "Int":
		integer := &resolve.Integer{
			Nullable: fieldTypeIsNullable,
		}
		if !isList {
			integer.Path = path
			v.Defer(func() {
				if override, ok := v.fieldPathOverrides[ref]; ok {
					integer.Path = override(integer.Path)
				}
			})
		}
		value = integer
	case "Float":
		float := &resolve.Float{
			Nullable: fieldTypeIsNullable,
		}
		if !isList {
			float.Path = path
			v.Defer(func() {
				if override, ok := v.fieldPathOverrides[ref]; ok {
					float.Path = override(float.Path)
				}
			})
		}
		value = float
	default:

		switch v.Definition.Index.Nodes[xxhash.Sum64(typeNameBytes)].Kind { // TODO verify definition type before and define resolve type based on that, in case of scalar use specific scalars and default to string
		case ast.NodeKindEnumTypeDefinition, ast.NodeKindScalarTypeDefinition:
			str := &resolve.String{
				Nullable: fieldTypeIsNullable,
			}
			if !isList {
				str.Path = path
				v.Defer(func() {
					if override, ok := v.fieldPathOverrides[ref]; ok {
						str.Path = override(str.Path)
					}
				})
			}
			value = str
		default:
			obj := &resolve.Object{
				Nullable: fieldTypeIsNullable,
			}
			if !isList {
				obj.Path = path
				v.Defer(func() {
					if override, ok := v.fieldPathOverrides[ref]; ok {
						obj.Path = override(obj.Path)
					}
				})
			}
			value = obj
			nextCurrentObject = obj
		}
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
				Path:     path,
				Item:     value,
				Nullable: fieldTypeIsNullable,
			}
			v.currentArray = list
			value = list
			if override, ok := v.fieldPathOverrides[ref]; ok {
				list.Path = override(list.Path)
			}
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
	for i := range v.Config.FieldMappings {
		if v.Config.FieldMappings[i].TypeName == typeName && v.Config.FieldMappings[i].FieldName == fieldName {
			if v.Config.FieldMappings[i].Path != nil {
				return v.Config.FieldMappings[i].Path
			}
			if v.Config.FieldMappings[i].DisableDefaultMapping {
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
	for i := range v.Config.DataSourceConfigurations {
		if v.Config.DataSourceConfigurations[i].TypeName != enclosingTypeName {
			continue
		}
		for j := range v.Config.DataSourceConfigurations[i].FieldNames {
			if v.Config.DataSourceConfigurations[i].FieldNames[j] == fieldName {
				v.activeDataSourcePlanner = v.Config.DataSourceConfigurations[i].DataSourcePlanner
				v.fieldDataSourcePlanners = append(v.fieldDataSourcePlanners, fieldDataSourcePlanner{
					field:              fieldRef,
					planner:            v.Config.DataSourceConfigurations[i].DataSourcePlanner,
					upstreamIdentifier: v.Config.DataSourceConfigurations[i].UpstreamUniqueIdentifier,
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
