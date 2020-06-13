package httpjsondatasource

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/buger/jsonparser"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/httpclient"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
)

type Planner struct {
	client httpclient.Client
	v      *plan.Visitor
}

func NewPlanner(client httpclient.Client) *Planner {
	return &Planner{
		client: client,
	}
}

func (p *Planner) clientOrDefault() httpclient.Client {
	if p.client != nil {
		return p.client
	}
	return httpclient.NewFastHttpClient(httpclient.DefaultFastHttpClient)
}

func (p *Planner) Register(visitor *plan.Visitor) {
	p.v = visitor
	visitor.RegisterEnterFieldVisitor(p)
}

func (p *Planner) EnterField(ref int) {
	rootField, config := p.v.IsRootField(ref)
	if !rootField {
		return
	}

	path := config.Attributes.ValueForKey(httpclient.PATH)
	baseURL := config.Attributes.ValueForKey(httpclient.BASEURL)
	method := config.Attributes.ValueForKey(httpclient.METHOD)
	body := config.Attributes.ValueForKey(httpclient.BODY)
	headers := config.Attributes.ValueForKey(httpclient.HEADERS)
	queryParams := config.Attributes.ValueForKey(httpclient.QUERYPARAMS)

	queryParams = p.prepareQueryParams(ref, queryParams)

	var (
		input []byte
	)

	url := []byte(string(baseURL) + string(path))

	input = httpclient.SetInputURL(input, url)
	input = httpclient.SetInputMethod(input, method)
	input = httpclient.SetInputBody(input, body)
	input = httpclient.SetInputHeaders(input, headers)
	input = httpclient.SetInputQueryParams(input, queryParams)

	bufferID := p.v.NextBufferID()
	p.v.SetBufferIDForCurrentFieldSet(bufferID)
	p.v.SetCurrentObjectFetch(&resolve.SingleFetch{
		BufferId: bufferID,
		Input:    input,
		DataSource: &Source{
			client: p.clientOrDefault(),
		},
		DisallowSingleFlight: !bytes.Equal(method, []byte("GET")),
	}, config)
}

type QueryValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func NewQueryValues(values ...QueryValue) []byte {
	out, _ := json.Marshal(values)
	return out
}

var (
	selectorRegex = regexp.MustCompile(`"{{\s(.*?)\s}}"`)
)

// prepareQueryParams ensures that values
func (p *Planner) prepareQueryParams(field int, params []byte) []byte {
	var values [][]byte
	_, err := jsonparser.ArrayEach(params, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
		values = append(values, value)
	})
	if err != nil {
		return params
	}

	for i := range values {
		values[i] = selectorRegex.ReplaceAllFunc(values[i], func(i []byte) []byte {
			subs := selectorRegex.FindSubmatch(i)
			if len(subs) != 2 {
				return i
			}
			path := string(bytes.TrimPrefix(subs[1], []byte(".")))
			segments := strings.Split(path, ".")
			if len(segments) < 2 {
				return i
			}
			argName := []byte(segments[1])
			if argRef, ok := p.v.Operation.FieldArgument(field, argName); ok {
				value := p.v.Operation.ArgumentValue(argRef)
				switch value.Kind {
				case ast.ValueKindVariable:
					variableName := p.v.Operation.VariableValueNameBytes(value.Ref)
					if variableDefinition, ok := p.v.Operation.VariableDefinitionByNameAndOperation(p.v.Ancestors[0].Ref, variableName); ok {
						typeRef := p.v.Operation.VariableDefinitions[variableDefinition].Type
						if p.v.Operation.TypeIsScalar(typeRef,p.v.Definition){
							return i
						}
						return i[1 : len(i)-1]
					}

				}
			}

			return i
		})
	}

	joined := bytes.Join(values, literal.COMMA)
	return append([]byte("["), append(joined, []byte("]")...)...)
}

type Source struct {
	client httpclient.Client
}

var (
	uniqueIdentifier = []byte("http_json")
)

func (_ *Source) UniqueIdentifier() []byte {
	return uniqueIdentifier
}

func (s *Source) Load(ctx context.Context, input []byte, bufPair *resolve.BufPair) (err error) {
	return s.client.Do(ctx, input, bufPair.Data)
}
