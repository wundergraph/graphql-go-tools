package rest_datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/TykTechnologies/graphql-go-tools/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/datasource/httpclient"
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/plan"
	"github.com/TykTechnologies/graphql-go-tools/pkg/lexer/literal"
)

type Planner struct {
	client              *http.Client
	v                   *plan.Visitor
	config              Configuration
	rootField           int
	operationDefinition int
	argumentTypeMap     map[string]int
	Operation           *ast.Document
}

func (p *Planner) EnterDocument(operation, definition *ast.Document) {
	p.Operation = operation
}

func (p *Planner) LeaveDocument(operation, definition *ast.Document) {
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

type Factory struct {
	Client *http.Client
}

func (f *Factory) Planner(ctx context.Context) plan.DataSourcePlanner {
	return &Planner{
		client:          f.Client,
		argumentTypeMap: map[string]int{},
	}
}

type Configuration struct {
	Fetch        FetchConfiguration
	Subscription SubscriptionConfiguration
}

func ConfigJSON(config Configuration) json.RawMessage {
	out, _ := json.Marshal(config)
	return out
}

type SubscriptionConfiguration struct {
	PollingIntervalMillis   int64
	SkipPublishSameResponse bool
}

type FetchConfiguration struct {
	URL    string
	Method string
	Header http.Header
	Query  []QueryConfiguration
	Body   string
}

type QueryConfiguration struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func (p *Planner) Register(visitor *plan.Visitor, configuration plan.DataSourceConfiguration, isNested bool) error {
	p.v = visitor
	visitor.Walker.RegisterDocumentVisitor(p)
	visitor.Walker.RegisterEnterFieldVisitor(p)
	visitor.Walker.RegisterEnterOperationVisitor(p)
	visitor.Walker.RegisterEnterArgumentVisitor(p)
	return json.Unmarshal(configuration.Custom, &p.config)
}

func (p *Planner) EnterField(ref int) {
	p.rootField = ref
}

func (p *Planner) EnterArgument(ref int) {
	fieldName := p.Operation.FieldNameString(p.rootField)
	argumentName := p.Operation.ArgumentNameString(ref)
	key := fmt.Sprintf("%s_%s", fieldName, argumentName)
	fmt.Println(key)
	val := p.Operation.Arguments[ref].Value
	if val.Kind == ast.ValueKindVariable {
		if !p.Operation.OperationDefinitionHasVariableDefinition(p.operationDefinition, p.Operation.VariableValueNameString(val.Ref)) {
			return
		}
		variableDefinition, exists := p.Operation.VariableDefinitionByNameAndOperation(p.operationDefinition, p.Operation.VariableValueNameBytes(val.Ref))
		if !exists {
			return
		}
		p.argumentTypeMap[key] = p.Operation.VariableDefinitions[variableDefinition].Type
		return
	}
}

func (p *Planner) configureInput() []byte {

	input := httpclient.SetInputURL(nil, []byte(p.config.Fetch.URL))
	input = httpclient.SetInputMethod(input, []byte(p.config.Fetch.Method))
	input = httpclient.SetInputBody(input, []byte(p.config.Fetch.Body))

	header, err := json.Marshal(p.config.Fetch.Header)
	if err == nil && len(header) != 0 && !bytes.Equal(header, literal.NULL) {
		input = httpclient.SetInputHeader(input, header)
	}

	preparedQuery := p.prepareQueryParams(p.rootField, p.config.Fetch.Query)
	query, err := json.Marshal(preparedQuery)
	if err == nil && len(preparedQuery) != 0 {
		input = httpclient.SetInputQueryParams(input, query)
	}
	return input
}

func (p *Planner) ConfigureFetch() plan.FetchConfiguration {
	input := p.configureInput()
	return plan.FetchConfiguration{
		Input: string(input),
		DataSource: &Source{
			client: p.client,
		},
		DisallowSingleFlight: p.config.Fetch.Method != "GET",
		DisableDataLoader:    true,
	}
}

func (p *Planner) ConfigureSubscription() plan.SubscriptionConfiguration {
	return plan.SubscriptionConfiguration{}
}

var (
	selectorRegex = regexp.MustCompile(`{{\s(.*?)\s}}`)
)

func (p *Planner) prepareQueryParams(field int, query []QueryConfiguration) []QueryConfiguration {
	out := make([]QueryConfiguration, 0, len(query))
Next:
	for i := range query {
		matches := selectorRegex.FindAllStringSubmatch(query[i].Value, -1)
		for j := range matches {
			if len(matches[j]) == 2 {
				path := matches[j][1]
				path = strings.TrimPrefix(path, ".")
				elements := strings.Split(path, ".")
				if len(elements) < 2 {
					continue
				}
				if elements[0] != "arguments" {
					continue
				}
				argumentName := elements[1]
				arg, ok := p.v.Operation.FieldArgument(field, []byte(argumentName))
				if !ok {
					continue Next
				}
				value := p.v.Operation.Arguments[arg].Value
				if value.Kind != ast.ValueKindVariable {
					continue Next
				}
				variableName := p.v.Operation.VariableValueNameString(value.Ref)
				if !p.v.Operation.OperationDefinitionHasVariableDefinition(p.operationDefinition, variableName) {
					continue Next
				}
			}
		}
		out = append(out, query[i])
	}
	return out
}

type Source struct {
	client *http.Client
}

func (s *Source) Load(ctx context.Context, input []byte, w io.Writer) (err error) {
	return httpclient.Do(s.client, ctx, input, w)
}
