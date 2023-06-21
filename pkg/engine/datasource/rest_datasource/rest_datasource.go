package rest_datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/buger/jsonparser"
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
}

const (
	typeString = "String"
)

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
		client: f.Client,
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
	Name       string `json:"name"`
	Value      string `json:"value"`
	rawMessage json.RawMessage
}

func (p *Planner) Register(visitor *plan.Visitor, configuration plan.DataSourceConfiguration, isNested bool) error {
	p.v = visitor
	visitor.Walker.RegisterEnterFieldVisitor(p)
	visitor.Walker.RegisterEnterOperationVisitor(p)
	return json.Unmarshal(configuration.Custom, &p.config)
}

func (p *Planner) EnterField(ref int) {
	p.rootField = ref
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
	query, err := p.marshalQueryParams(preparedQuery)
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
	selectorRegex = regexp.MustCompile(`{{\s?(.*?)\s?}}`)
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

				variableDefRef, exists := p.v.Operation.VariableDefinitionByNameAndOperation(p.operationDefinition, p.v.Operation.VariableValueNameBytes(value.Ref))
				if !exists {
					continue
				}
				typeRef := p.v.Operation.VariableDefinitions[variableDefRef].Type
				typeName := p.v.Operation.TypeNameString(typeRef)
				typeKind := p.v.Operation.Types[typeRef].TypeKind
				// if type is a nullable or non-nullable string, add quotes to the raw message
				if typeName == typeString || (typeKind == ast.TypeKindNonNull && p.v.Operation.TypeNameString(p.v.Operation.Types[typeRef].OfType) == typeString) {
					query[i].rawMessage = []byte(`"` + query[i].Value + `"`)
				} else {
					query[i].rawMessage = []byte(query[i].Value)
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

func (p *Planner) marshalQueryParams(params []QueryConfiguration) ([]byte, error) {
	marshalled, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	for i := range params {
		if params[i].rawMessage != nil {
			marshalled, err = jsonparser.Set(marshalled, params[i].rawMessage, fmt.Sprintf("[%d]", i), "value")
		} else {
			marshalled, err = jsonparser.Set(marshalled, []byte(`"`+params[i].Value+`"`), fmt.Sprintf("[%d]", i), "value")
		}
		if err != nil {
			return nil, err
		}
	}
	return marshalled, nil
}

type Source struct {
	client *http.Client
}

func (s *Source) Load(ctx context.Context, input []byte, w io.Writer) (err error) {
	return httpclient.Do(s.client, ctx, input, w)
}
