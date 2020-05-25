package fasthttpjsondatasource

import (
	"context"
	"time"

	"github.com/buger/jsonparser"
	"github.com/tidwall/sjson"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"

	"github.com/valyala/fasthttp"
)

const (
	URL     = "url"
	BASEURL = "base_url"
	METHOD  = "method"
	BODY    = "body"
	HEADERS = "headers"
)

type Planner struct {
	client *fasthttp.Client
	v      *plan.Visitor
}

func NewPlanner(client *fasthttp.Client) *Planner {
	return &Planner{
		client: client,
	}
}

func (p *Planner) getClient() *fasthttp.Client {
	if p.client != nil {
		return p.client
	}
	return &fasthttp.Client{
		WriteTimeout: time.Second * 5,
		ReadTimeout:  time.Second * 5,
	}
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

	url := config.Attributes.ValueForKey(URL)
	baseURL := config.Attributes.ValueForKey(BASEURL)
	method := config.Attributes.ValueForKey(METHOD)
	body := config.Attributes.ValueForKey(BODY)
	headers := config.Attributes.ValueForKey(HEADERS)

	var (
		input []byte
		err   error
	)

	url = append(baseURL, url...)

	if url != nil {
		input, err = sjson.SetBytes(input, URL, string(url))
	}
	if method != nil {
		input, err = sjson.SetBytes(input, METHOD, string(method))
	}
	if body != nil {
		input, err = sjson.SetRawBytes(input, BODY, body)
	}
	if headers != nil {
		input, err = sjson.SetRawBytes(input, HEADERS, headers)
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
			client: p.getClient(),
		},
	}, config)
}

type Source struct {
	client *fasthttp.Client
}

var (
	uniqueIdentifier = []byte("fast_http_json")
)

func (_ *Source) UniqueIdentifier() []byte {
	return uniqueIdentifier
}

var (
	accept          = []byte("Accept")
	applicationJSON = []byte("application/json")
)

func (s *Source) Load(ctx context.Context, input []byte, bufPair *resolve.BufPair) (err error) {

	var (
		url, method, body, headers []byte
		inputPaths                 = [][]string{
			{URL},
			{METHOD},
			{BODY},
			{HEADERS},
		}
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
		}
	}, inputPaths...)

	req, res := fasthttp.AcquireRequest(), fasthttp.AcquireResponse()
	defer func() {
		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(res)
	}()

	req.Header.SetMethodBytes(method)
	req.SetRequestURIBytes(url)
	req.SetBody(body)

	err = jsonparser.ObjectEach(headers, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
		req.Header.SetBytesKV(key, value)
		return nil
	})

	req.Header.AddBytesKV(accept, applicationJSON)

	if deadline, ok := ctx.Deadline(); ok {
		err = s.client.DoDeadline(req, res, deadline)
	} else {
		err = s.client.Do(req, res)
	}

	if err != nil {
		return
	}

	_, err = bufPair.Data.Write(res.Body())
	return
}
