package introspection_datasource

import (
	"bytes"
	"encoding/json"
	"strconv"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/introspection"
)

type Planner struct {
	introspectionData *introspection.Data
	v                 *plan.Visitor
	rootField         int
}

func (p *Planner) DownstreamResponseFieldAlias(_ int) (alias string, exists bool) {
	// the Introspection DataSourcePlanner doesn't rewrite upstream fields: skip
	return
}

func (p *Planner) DataSourcePlanningBehavior() plan.DataSourcePlanningBehavior {
	return plan.DataSourcePlanningBehavior{
		MergeAliasedRootNodes:      false,
		OverrideFieldPathFromAlias: false,
	}
}

func (p *Planner) Register(visitor *plan.Visitor, _ json.RawMessage, _ bool) error {
	p.v = visitor
	visitor.Walker.RegisterEnterFieldVisitor(p)
	return nil
}

func (p *Planner) EnterField(ref int) {
	p.rootField = ref
}

func (p *Planner) configureInput() string {
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

	return buf.String()
}

func (p *Planner) ConfigureFetch() plan.FetchConfiguration {
	input := p.configureInput()
	return plan.FetchConfiguration{
		Input: input,
		DataSource: &Source{
			introspectionData: p.introspectionData,
		},
	}
}

func (p *Planner) ConfigureSubscription() plan.SubscriptionConfiguration {
	// the Introspection DataSourcePlanner doesn't have subscription
	return plan.SubscriptionConfiguration{}
}
