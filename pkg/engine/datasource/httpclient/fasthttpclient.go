package httpclient

import (
	"context"
	"io"
	"time"

	"github.com/buger/jsonparser"
	"github.com/valyala/fasthttp"
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
	queryParamsKeys = [][]string{
		{"name"},
		{"value"},
	}
	applicationJsonBytes = []byte("application/json")
	contentTypeBytes     = []byte("Content-Type")
	acceptBytes          = []byte("Accept")
)

func (f *FastHttpClient) Do(ctx context.Context, requestInput []byte, out io.Writer) (err error) {

	url, method, body, headers, queryParams := requestInputParams(requestInput)

	req, res := fasthttp.AcquireRequest(), fasthttp.AcquireResponse()
	defer func() {
		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(res)
	}()

	req.Header.SetMethodBytes(method)
	req.SetRequestURIBytes(url)
	req.SetBody(body)

	if headers != nil {
		err = jsonparser.ObjectEach(headers, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
			req.Header.SetBytesKV(key, value)
			return nil
		})
		if err != nil {
			return err
		}
	}

	req.Header.AddBytesKV(contentTypeBytes, applicationJsonBytes)
	req.Header.AddBytesKV(acceptBytes, applicationJsonBytes)

	if queryParams != nil {
		_, err = jsonparser.ArrayEach(queryParams, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
			var (
				parameterName, parameterValue []byte
			)
			jsonparser.EachKey(value, func(i int, bytes []byte, valueType jsonparser.ValueType, err error) {
				switch i {
				case 0:
					parameterName = bytes
				case 1:
					parameterValue = bytes
				}
			}, queryParamsKeys...)
			if parameterName != nil && parameterValue != nil {
				req.URI().QueryArgs().AddBytesKV(parameterName, parameterValue)
			}
		})
		if err != nil {
			return err
		}
	}

	req.Header.AddBytesKV(acceptBytes, applicationJsonBytes)

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
