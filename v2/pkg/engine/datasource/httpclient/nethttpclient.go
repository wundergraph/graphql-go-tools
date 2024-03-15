package httpclient

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/buger/jsonparser"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/lexer/literal"
)

const (
	ContentEncodingHeader = "Content-Encoding"
	AcceptEncodingHeader  = "Accept-Encoding"
	AcceptHeader          = "Accept"
	ContentTypeHeader     = "Content-Type"

	EncodingGzip    = "gzip"
	EncodingDeflate = "deflate"
	EncodingBrotli  = "br"

	ContentTypeJSON = "application/json"
)

var (
	DefaultNetHttpClient = &http.Client{
		Timeout: time.Second * 10,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 1024,
			TLSHandshakeTimeout: 0 * time.Second,
		},
	}
	queryParamsKeys = [][]string{
		{"name"},
		{"value"},
	}
)

type TraceHTTP struct {
	Request  TraceHTTPRequest  `json:"request"`
	Response TraceHTTPResponse `json:"response"`
}

type TraceHTTPRequest struct {
	Method  string      `json:"method"`
	URL     string      `json:"url"`
	Headers http.Header `json:"headers"`
}

type TraceHTTPResponse struct {
	StatusCode int         `json:"status_code"`
	Status     string      `json:"status"`
	Headers    http.Header `json:"headers"`
	BodySize   int         `json:"body_size"`
}

func Do(client *http.Client, ctx context.Context, requestInput []byte, out io.Writer) (err error) {

	url, method, body, headers, queryParams, enableTrace := requestInputParams(requestInput)

	request, err := http.NewRequestWithContext(ctx, string(method), string(url), bytes.NewReader(body))
	if err != nil {
		return err
	}

	if headers != nil {
		err = jsonparser.ObjectEach(headers, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
			_, err := jsonparser.ArrayEach(value, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
				if err != nil {
					return
				}
				if len(value) == 0 {
					return
				}
				request.Header.Add(string(key), string(value))
			})
			return err
		})
		if err != nil {
			return err
		}
	}

	if queryParams != nil {
		query := request.URL.Query()
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
			if len(parameterName) != 0 && len(parameterValue) != 0 {
				if bytes.Equal(parameterValue[:1], literal.LBRACK) {
					_, _ = jsonparser.ArrayEach(parameterValue, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
						query.Add(string(parameterName), string(value))
					})
				} else {
					query.Add(string(parameterName), string(parameterValue))
				}
			}
		})
		if err != nil {
			return err
		}
		request.URL.RawQuery = query.Encode()
	}

	request.Header.Add(AcceptHeader, ContentTypeJSON)
	request.Header.Add(ContentTypeHeader, ContentTypeJSON)
	request.Header.Set(AcceptEncodingHeader, EncodingGzip)
	request.Header.Add(AcceptEncodingHeader, EncodingDeflate)
	request.Header.Add(AcceptEncodingHeader, EncodingBrotli)

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	respReader, err := respBodyReader(response)
	if err != nil {
		return err
	}

	if !enableTrace {
		_, err = io.Copy(out, respReader)
		return
	}

	buf := &bytes.Buffer{}
	_, err = io.Copy(buf, respReader)
	if err != nil {
		return err
	}
	responseTrace := TraceHTTP{
		Request: TraceHTTPRequest{
			Method:  request.Method,
			URL:     request.URL.String(),
			Headers: redactHeaders(request.Header),
		},
		Response: TraceHTTPResponse{
			StatusCode: response.StatusCode,
			Status:     response.Status,
			Headers:    redactHeaders(response.Header),
			BodySize:   buf.Len(),
		},
	}
	trace, err := json.Marshal(responseTrace)
	if err != nil {
		return err
	}
	responseWithTraceExtension, err := jsonparser.Set(buf.Bytes(), trace, "extensions", "trace")
	if err != nil {
		return err
	}
	_, err = out.Write(responseWithTraceExtension)
	return err
}

var headersToRedact = []string{
	"authorization",
	"www-authenticate",
	"proxy-authenticate",
	"proxy-authorization",
	"cookie",
	"set-cookie",
}

func redactHeaders(headers http.Header) http.Header {
	redactedHeaders := make(http.Header)
	for key, values := range headers {
		if slices.Contains(headersToRedact, strings.ToLower(key)) {
			redactedHeaders[key] = []string{"****"}
		} else {
			redactedHeaders[key] = values
		}
	}
	return redactedHeaders
}

var (
	ErrNonOkResponse = errors.New("server returned non 200 OK response")
)

func respBodyReader(res *http.Response) (io.Reader, error) {
	if res.StatusCode != http.StatusOK {
		return nil, ErrNonOkResponse
	}
	switch res.Header.Get(ContentEncodingHeader) {
	case EncodingGzip:
		return gzip.NewReader(res.Body)
	case EncodingDeflate:
		return flate.NewReader(res.Body), nil
	case EncodingBrotli:
		return io.NopCloser(brotli.NewReader(res.Body)), nil
	default:
		return res.Body, nil
	}
}
