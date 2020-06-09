package httpjsondatasource

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/httpclient"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
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
