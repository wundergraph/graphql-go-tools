package httpclient

import (
	"bufio"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/buger/jsonparser"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
)

const (
	ContentEncodingHeader = "Content-Encoding"
	AcceptEncodingHeader  = "Accept-Encoding"
	AcceptHeader          = "Accept"
	ContentTypeHeader     = "Content-Type"
	ContentLengthHeader   = "Content-Length"

	EncodingGzip    = "gzip"
	EncodingDeflate = "deflate"

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

type responseContextKey struct{}

type ResponseContext struct {
	StatusCode int
	Request    *http.Request
	Response   *http.Response
}

func InjectResponseContext(ctx context.Context) (context.Context, *ResponseContext) {
	value := &ResponseContext{}
	return context.WithValue(ctx, responseContextKey{}, value), value
}

func setRequest(ctx context.Context, request *http.Request) {
	if value, ok := ctx.Value(responseContextKey{}).(*ResponseContext); ok {
		value.Request = request
	}
}

func setResponseStatus(ctx context.Context, request *http.Request, response *http.Response) {
	if value, ok := ctx.Value(responseContextKey{}).(*ResponseContext); ok {
		if response != nil {
			value.StatusCode = response.StatusCode
		} else {
			value.StatusCode = 0
		}
		value.Request = request
		value.Response = response
	}
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

func respBodyReader(res *http.Response) (io.Reader, error) {
	switch res.Header.Get(ContentEncodingHeader) {
	case EncodingGzip:
		return gzip.NewReader(res.Body)
	case EncodingDeflate:
		return flate.NewReader(res.Body), nil
	default:
		return res.Body, nil
	}
}

type httpClientContext string

const (
	sizeHintKey httpClientContext = "size-hint"
)

// WithHTTPClientSizeHint allows the engine to keep track of response sizes per subgraph fetch
// If a hint is supplied, we can create a buffer of size close to the required size
// This reduces allocations by reducing the buffer grow calls, which always copies the buffer
func WithHTTPClientSizeHint(ctx context.Context, size int) context.Context {
	return context.WithValue(ctx, sizeHintKey, size)
}

func buffer(ctx context.Context) *bytes.Buffer {
	if sizeHint, ok := ctx.Value(sizeHintKey).(int); ok && sizeHint > 0 {
		return bytes.NewBuffer(make([]byte, 0, sizeHint))
	}
	// if we start with zero, doubling will take a while until we reach the required size
	// if we start with a high number, e.g. 1024, we just increase the memory usage of the engine
	// 64 seems to be a healthy middle ground
	return bytes.NewBuffer(make([]byte, 0, 64))
}

func makeHTTPRequest(client *http.Client, ctx context.Context, baseHeaders http.Header, url, method, headers, queryParams []byte, body io.Reader, enableTrace bool, contentType string, contentLength int) ([]byte, error) {

	request, err := http.NewRequestWithContext(ctx, string(method), string(url), body)
	if err != nil {
		return nil, err
	}

	if baseHeaders != nil {
		request.Header = baseHeaders
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
			return nil, err
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
			return nil, err
		}
		request.URL.RawQuery = query.Encode()
	}

	request.Header.Add(AcceptHeader, ContentTypeJSON)
	request.Header.Add(ContentTypeHeader, contentType)
	request.Header.Set(AcceptEncodingHeader, EncodingGzip)
	request.Header.Add(AcceptEncodingHeader, EncodingDeflate)
	if contentLength > 0 {
		// always set the ContentLength field so that chunking can be avoided
		// and other parties can more efficiently parse
		request.ContentLength = int64(contentLength)
	}

	setRequest(ctx, request)

	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	setResponseStatus(ctx, request, response)

	respReader, err := respBodyReader(response)
	if err != nil {
		return nil, err
	}

	// we intentionally don't use a pool of sorts here
	// we're buffering the response and then later, in the engine,
	// parse it into an JSON AST with the use of an arena, which is quite efficient
	// Through trial and error it turned out that it's best to leave this buffer to the GC
	// It'll know best the lifecycle of the buffer
	// Using an arena here just increased overall memory usage
	out := buffer(ctx)
	_, err = out.ReadFrom(respReader)
	if err != nil {
		return nil, err
	}

	if !enableTrace {
		return out.Bytes(), nil
	}

	data := out.Bytes()
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
			BodySize:   len(data),
		},
	}
	trace, err := json.Marshal(responseTrace)
	if err != nil {
		return nil, err
	}
	responseWithTraceExtension, err := jsonparser.Set(data, trace, "extensions", "trace")
	if err != nil {
		return nil, err
	}
	return responseWithTraceExtension, nil
}

func Do(client *http.Client, ctx context.Context, baseHeaders http.Header, requestInput []byte) (data []byte, err error) {
	url, method, body, headers, queryParams, enableTrace := requestInputParams(requestInput)
	return makeHTTPRequest(client, ctx, baseHeaders, url, method, headers, queryParams, bytes.NewReader(body), enableTrace, ContentTypeJSON, len(body))
}

func DoMultipartForm(
	client *http.Client, ctx context.Context, baseHeaders http.Header, requestInput []byte, files []*FileUpload,
) (data []byte, err error) {
	if len(files) == 0 {
		return nil, errors.New("no files provided")
	}

	url, method, body, headers, queryParams, enableTrace := requestInputParams(requestInput)

	formValues := map[string]io.Reader{
		"operations": bytes.NewReader(body),
	}

	var tempFiles []*os.File

	fileMap := bytes.NewBuffer(nil)
	fileMap.WriteString("{")
	hasWrittenFileName := false

	for i, file := range files {
		if hasWrittenFileName {
			fileMap.WriteString(",")
		}
		hasWrittenFileName = true

		_, _ = fmt.Fprintf(fileMap, `"%d":["%s"]`, i, file.variablePath)

		key := fmt.Sprintf("%d", i)
		temporaryFile, err := os.Open(file.Path())
		tempFiles = append(tempFiles, temporaryFile)
		if err != nil {
			return nil, err
		}
		formValues[key] = bufio.NewReader(temporaryFile)
	}
	fileMap.WriteString("}")
	formValues["map"] = strings.NewReader(fileMap.String())

	multipartBody, contentType, err := multipartBytes(formValues, files)
	if err != nil {
		return nil, err
	}

	defer func() {
		multipartBody.Close()
		for _, file := range tempFiles {
			if err := file.Close(); err != nil {
				return
			}
			if err = os.Remove(file.Name()); err != nil {
				return
			}
		}
	}()

	return makeHTTPRequest(client, ctx, baseHeaders, url, method, headers, queryParams, multipartBody, enableTrace, contentType, 0)
}

func multipartBytes(values map[string]io.Reader, files []*FileUpload) (*io.PipeReader, string, error) {
	byteBuf := &bytes.Buffer{}
	mpWriter := multipart.NewWriter(byteBuf)
	contentType := mpWriter.FormDataContentType()

	// First create the fields to control the file upload
	valuesInOrder := []string{"operations", "map"}
	for _, key := range valuesInOrder {
		r := values[key]
		fw, err := mpWriter.CreateFormField(key)
		if err != nil {
			return nil, contentType, err
		}
		if _, err = io.Copy(fw, r); err != nil {
			return nil, contentType, err
		}
	}

	// Insert parts for files
	boundaries := make([][]byte, 0, len(files))
	for i, file := range files {
		key := fmt.Sprintf("%d", i)
		_, err := mpWriter.CreateFormFile(key, file.Name())
		if err != nil {
			return nil, contentType, err
		}

		// We read the files using pipe later
		// So we need to keep store boundaries to insert contents in the correct place
		lengthOfBufferTillBoundary := byteBuf.Len()
		boundary := make([]byte, lengthOfBufferTillBoundary)
		if _, err = byteBuf.Read(boundary); err != nil {
			return nil, contentType, err
		}
		boundaries = append(boundaries, boundary)
	}

	err := mpWriter.Close()
	if err != nil {
		return nil, contentType, err
	}

	rd, wr := io.Pipe()

	go func() {
		defer func() {
			err := wr.Close()
			if err != nil {
				fmt.Println("Error closing pipe: ", err)
			}
		}()

		// 4MB chunks
		buf := make([]byte, 2048*2048)
		for i, file := range files {
			if _, err = wr.Write(boundaries[i]); err != nil {
				return
			}

			f, err := os.Open(file.Path())
			if err != nil {
				return
			}

			for {
				n, err := f.Read(buf)
				if err != nil && err == io.EOF {
					break
				} else if err != nil {
					return
				}

				if _, err = wr.Write(buf[:n]); err != nil {
					return
				}
			}
			if err := f.Close(); err != nil {
				return
			}
		}
		// Write last boundary
		_, _ = wr.Write(byteBuf.Bytes())
	}()

	return rd, contentType, nil
}
