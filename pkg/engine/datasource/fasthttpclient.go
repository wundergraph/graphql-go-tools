package datasource

import (
	"context"
	"io"
	"time"

	"github.com/buger/jsonparser"
	"github.com/valyala/fasthttp"
)

var (
	accept          = []byte("Accept")
	applicationJSON = []byte("application/json")
)

type FastHttpClient struct {
	client *fasthttp.Client
}

func NewFastHttpClient(client *fasthttp.Client) *FastHttpClient {
	return &FastHttpClient{
		client: client,
	}
}

var (
	DefaultFastHttpClient = &fasthttp.Client{
		ReadTimeout:         time.Second * 10,
		WriteTimeout:        time.Second * 10,
		MaxIdleConnDuration: time.Minute,
	}
)

func (f *FastHttpClient) Do(ctx context.Context, url, method, headers, body []byte, out io.Writer) (err error) {
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
		err = f.client.DoDeadline(req, res, deadline)
	} else {
		err = f.client.Do(req, res)
	}

	if err != nil {
		return
	}

	return res.BodyWriteTo(out)
}
