package httpclient

import (
	"bytes"
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
	acceptBytes          = []byte("accept")
	acceptEncodingBytes  = []byte("Accept-Encoding")
	gzipEncodingBytes    = []byte("gzip")
	userAgentBytes       = []byte("graphql-go-client")
	contentEncoding      = []byte("Content-Encoding")
)

func (f *FastHttpClient) Do(ctx context.Context, requestInput []byte, out io.Writer) (err error) {

	url, method, body, headers, queryParams := requestInputParams(requestInput)

	req, res := fasthttp.AcquireRequest(), fasthttp.AcquireResponse()
	defer func() {
		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(res)
	}()

	req.Header.SetUserAgentBytes(userAgentBytes)
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

				_, arrayParseErr := jsonparser.ArrayEach(parameterValue, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
					req.URI().QueryArgs().AddBytesKV(parameterName, value)
				})
				if arrayParseErr != nil {
					req.URI().QueryArgs().AddBytesKV(parameterName, parameterValue)
				}
			}
		})
		if err != nil {
			return err
		}
	}

	req.Header.SetBytesKV(acceptBytes, applicationJsonBytes)
	req.Header.SetBytesKV(acceptEncodingBytes, gzipEncodingBytes)
	req.Header.SetContentTypeBytes(applicationJsonBytes)

	if deadline, ok := ctx.Deadline(); ok {
		err = f.client.DoDeadline(req, res, deadline)
	} else {
		err = f.client.Do(req, res)
	}

	if err != nil {
		return
	}

	if bytes.Equal(res.Header.PeekBytes(contentEncoding), gzipEncodingBytes) {
		body, err := res.BodyGunzip()
		if err != nil {
			return err
		}
		_, err = out.Write(body)
		return err
	}

	return res.BodyWriteTo(out)
}
