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

type InrospectionRequest struct {
	RequestType IntrospectionRequestType `json:"request_type"`
	TypeName    *string                  `json:"type_name"`
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

	var objArg = []byte(`"onType":"{{ .object.name }}"`)
	var queryArg = []byte(`"Type":"{{ .arguments.name }}"`)
	var filterArg = []byte(`"filter":{{ .arguments.includeDeprecated }}`)

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
	return json.NewEncoder(w).Encode(s.IntrospectionData)
}
