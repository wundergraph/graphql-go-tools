package introspection_datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strconv"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/introspection"
)

type IntrospectionRequestType int

const (
	SchemaIntrospectionRequestType IntrospectionRequestType = iota + 1
	TypeIntrospectionRequestType
	TypeFieldsIntrospectionRequestType
	EnumValuesIntrospectionRequestType
)

type IntrospectionRequest struct {
	RequestType       IntrospectionRequestType `json:"request_type"`
	OnTypeName        *string                  `json:"on_type_name"`
	TypeName          *string                  `json:"type_name"`
	IncludeDeprecated bool                     `json:"include_deprecated"`
}

type Factory struct {
	IntrospectionData *introspection.Data
}

func (f *Factory) Planner(ctx context.Context) plan.DataSourcePlanner {
	return &Planner{IntrospectionData: f.IntrospectionData}
}

type Planner struct {
	IntrospectionData   *introspection.Data
	v                   *plan.Visitor
	rootField           int
	operationDefinition int
}

func (p *Planner) DownstreamResponseFieldAlias(_ int) (alias string, exists bool) {
	// the REST DataSourcePlanner doesn't rewrite upstream fields: skip
	return
}

func (p *Planner) DataSourcePlanningBehavior() plan.DataSourcePlanningBehavior {
	return plan.DataSourcePlanningBehavior{
		MergeAliasedRootNodes:      false,
		OverrideFieldPathFromAlias: false,
	}
}

func (p *Planner) EnterOperationDefinition(ref int) {
	p.operationDefinition = ref
}

func (p *Planner) Register(visitor *plan.Visitor, _ json.RawMessage, isNested bool) error {
	p.v = visitor
	visitor.Walker.RegisterEnterFieldVisitor(p)
	visitor.Walker.RegisterEnterOperationVisitor(p)
	return nil
}

func (p *Planner) EnterField(ref int) {
	p.rootField = ref
}

func (p *Planner) configureInput() []byte {
	fieldName := p.v.Operation.FieldNameString(p.rootField)

	var objArg = []byte(`"on_type_name":"{{ .object.name }}"`)
	var queryArg = []byte(`"type_name":"{{ .arguments.name }}"`)
	var filterArg = []byte(`"include_deprecated":{{ .arguments.includeDeprecated }}`)

	var (
		typeName     []byte
		fieldsFilter []byte
		requestType  = SchemaIntrospectionRequestType
	)
	switch fieldName {
	case "__type":
		requestType = TypeIntrospectionRequestType
		typeName = queryArg
	case "fields":
		requestType = TypeFieldsIntrospectionRequestType
		typeName = objArg
		fieldsFilter = filterArg
	case "enumValues":
		requestType = EnumValuesIntrospectionRequestType
		typeName = objArg
		fieldsFilter = filterArg
	}

	buf := bytes.Buffer{}
	buf.Write([]byte(`{"request_type":`))
	buf.Write([]byte(strconv.Itoa(int(requestType))))

	if typeName != nil {
		buf.Write([]byte(`,`))
		buf.Write(typeName)
	}

	if fieldsFilter != nil {
		buf.Write([]byte(`,`))
		buf.Write(fieldsFilter)
	}

	buf.Write([]byte(`}`))

	return buf.Bytes()
}

func (p *Planner) ConfigureFetch() plan.FetchConfiguration {
	input := p.configureInput()
	return plan.FetchConfiguration{
		Input: string(input),
		DataSource: &Source{
			IntrospectionData: p.IntrospectionData,
		},
	}
}

func (p *Planner) ConfigureSubscription() plan.SubscriptionConfiguration {
	return plan.SubscriptionConfiguration{}
}

type Source struct {
	IntrospectionData *introspection.Data
}

func (s *Source) Load(ctx context.Context, input []byte, w io.Writer) (err error) {
	println(string(input))

	input = bytes.Replace(input, []byte(`"include_deprecated":}`), []byte(`"include_deprecated":false}`), 1)

	var req IntrospectionRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return err
	}

	switch req.RequestType {
	case TypeIntrospectionRequestType:
		return s.singleType(w, *req.TypeName)
	case EnumValuesIntrospectionRequestType:
		return s.enumValuesForType(w, *req.OnTypeName, req.IncludeDeprecated)
	case TypeFieldsIntrospectionRequestType:
		return s.fieldsForType(w, *req.OnTypeName, req.IncludeDeprecated)
	}

	return json.NewEncoder(w).Encode(s.IntrospectionData)
}

func (s *Source) typeInfo(typeName string) *introspection.FullType {
	for _, fullType := range s.IntrospectionData.Schema.Types {
		if fullType.Name == typeName {
			return &fullType
		}
	}
	return nil
}

func (s *Source) singleType(w io.Writer, typeName string) error {
	typeInfo := s.typeInfo(typeName)
	return json.NewEncoder(w).Encode(typeInfo)
}

func (s *Source) fieldsForType(w io.Writer, typeName string, includeDeprecated bool) error {
	typeInfo := s.typeInfo(typeName)

	return json.NewEncoder(w).Encode(typeInfo.Fields)
}

func (s *Source) enumValuesForType(w io.Writer, typeName string, includeDeprecated bool) error {
	typeInfo := s.typeInfo(typeName)

	return json.NewEncoder(w).Encode(typeInfo.EnumValues)
}
