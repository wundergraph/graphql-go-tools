package httpjsondatasource

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"

	"github.com/buger/jsonparser"
	"github.com/tidwall/sjson"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafebytes"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
)

const (
	URL     = "url"
	BASEURL = "base_url"
	METHOD  = "method"
	BODY    = "body"
	HEADERS = "headers"
)

type Planner struct {
	client *http.Client
	v      *plan.Visitor
}

func NewPlanner(client *http.Client) *Planner {
	return &Planner{
		client: client,
	}
}

func (p *Planner) getClient() *http.Client {
	if p.client != nil {
		return p.client
	}
	return &http.Client{
		Timeout: time.Second * 10,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 1024,
			TLSHandshakeTimeout: 0,
		},
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
		DisallowSingleFlight: !bytes.Equal(method, []byte("GET")),
	}, config)
}

type Source struct {
	client *http.Client
}

var (
	uniqueIdentifier = []byte("http_json")
)

func (_ *Source) UniqueIdentifier() []byte {
	return uniqueIdentifier
}

func (s *Source) Load(ctx context.Context, input []byte, bufPair *resolve.BufPair) (err error) {

	var (
		url, method, body, headers []byte
		inputPaths                 = [][]string{
			{URL},
			{METHOD},
			{BODY},
			{HEADERS},
		}
		bodyReader io.Reader
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

	if len(body) != 0 {
		bodyReader = bytes.NewReader(body)
	}

	request, err := http.NewRequestWithContext(ctx, unsafebytes.BytesToString(method), unsafebytes.BytesToString(url), bodyReader)
	if err != nil {
		return err
	}

	if len(headers) != 0 {
		err = jsonparser.ObjectEach(headers, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
			request.Header.Add(unsafebytes.BytesToString(key), unsafebytes.BytesToString(value))
			return nil
		})
		if err != nil {
			return err
		}
	}

	request.Header.Add("Accept", "application/json")

	response, err := s.client.Do(request)
	if err != nil {
		return err
	}

	defer response.Body.Close()

	_, err = io.Copy(bufPair.Data, response.Body)
	if err != nil {
		return
	}

	return nil
}
