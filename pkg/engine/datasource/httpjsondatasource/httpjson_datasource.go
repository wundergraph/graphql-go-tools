package httpjsondatasource

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/buger/jsonparser"
	"github.com/tidwall/sjson"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
)

type Planner struct {
	client datasource.Client
	v      *plan.Visitor
}

func NewPlanner(client datasource.Client) *Planner {
	return &Planner{
		client: client,
	}
}

func (p *Planner) clientOrDefault() datasource.Client {
	if p.client != nil {
		return p.client
	}
	return datasource.NewFastHttpClient(datasource.DefaultFastHttpClient)
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

	path := config.Attributes.ValueForKey(datasource.PATH)
	baseURL := config.Attributes.ValueForKey(datasource.BASEURL)
	method := config.Attributes.ValueForKey(datasource.METHOD)
	body := config.Attributes.ValueForKey(datasource.BODY)
	headers := config.Attributes.ValueForKey(datasource.HEADERS)
	queryParams := config.Attributes.ValueForKey(datasource.QUERYPARAMS)

	var (
		input []byte
		err   error
	)

	url := append(baseURL, path...)

	if url != nil {
		input, err = sjson.SetBytes(input, datasource.URL, string(url))
	}
	if method != nil {
		input, err = sjson.SetBytes(input, datasource.METHOD, string(method))
	}
	if body != nil {
		input, err = sjson.SetRawBytes(input, datasource.BODY, body)
	}
	if headers != nil {
		input, err = sjson.SetRawBytes(input, datasource.HEADERS, headers)
	}
	if queryParams != nil {
		input, err = sjson.SetRawBytes(input, datasource.QUERYPARAMS, queryParams)
	}
	if err != nil {
		p.v.HandleInternalErr(err)
	}

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
	client datasource.Client
}

var (
	uniqueIdentifier = []byte("http_json")
	inputPaths       = [][]string{
		{datasource.URL},
		{datasource.METHOD},
		{datasource.BODY},
		{datasource.HEADERS},
		{datasource.QUERYPARAMS},
	}
)

func (_ *Source) UniqueIdentifier() []byte {
	return uniqueIdentifier
}

func (s *Source) Load(ctx context.Context, input []byte, bufPair *resolve.BufPair) (err error) {

	var (
		url, method, body, headers, queryParams []byte
	)

	jsonparser.EachKey(input, func(i int, bytes []byte, valueType jsonparser.ValueType, err error) {
		switch i {
		case 0:
			url = bytes
		case 1:
			method = bytes
		case 2:
			body = bytes
		case 3:
			headers = bytes
		case 4:
			queryParams = bytes
		}
	}, inputPaths...)

	return s.client.Do(ctx, url, queryParams, method, headers, body, bufPair.Data)
}
