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
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

const (
	ContentEncodingHeader = "Content-Encoding"
	AcceptEncodingHeader  = "Accept-Encoding"
	AcceptHeader          = "Accept"
	ContentTypeHeader     = "Content-Type"

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

func setResponseStatus(ctx context.Context, request *http.Request, response *http.Response) {
	if value, ok := ctx.Value(responseContextKey{}).(*ResponseContext); ok {
		value.StatusCode = response.StatusCode
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

type bodyHashContextKey struct{}

func BodyHashFromContext(ctx context.Context) (uint64, bool) {
	value := ctx.Value(bodyHashContextKey{})
	if value == nil {
		return 0, false
	}
	return value.(uint64), true
}

func makeHTTPRequest(client *http.Client, ctx context.Context, url, method, headers, queryParams []byte, body io.Reader, enableTrace bool, out *bytes.Buffer, contentType string) (err error) {

	request, err := http.NewRequestWithContext(ctx, string(method), string(url), body)
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
	request.Header.Add(ContentTypeHeader, contentType)
	request.Header.Set(AcceptEncodingHeader, EncodingGzip)
	request.Header.Add(AcceptEncodingHeader, EncodingDeflate)

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	request.Header = redactHeaders(request.Header)
	response.Header = redactHeaders(response.Header)

	setResponseStatus(ctx, request, response)

	respReader, err := respBodyReader(response)
	if err != nil {
		return err
	}

	if !enableTrace {
		if response.ContentLength > 0 {
			out.Grow(int(response.ContentLength))
		} else {
			out.Grow(1024 * 4)
		}
		_, err = out.ReadFrom(respReader)
		return
	}

	data, err := io.ReadAll(respReader)
	if err != nil {
		return err
	}
	responseTrace := TraceHTTP{
		Request: TraceHTTPRequest{
			Method:  request.Method,
			URL:     request.URL.String(),
			Headers: request.Header,
		},
		Response: TraceHTTPResponse{
			StatusCode: response.StatusCode,
			Status:     response.Status,
			Headers:    response.Header,
			BodySize:   len(data),
		},
	}
	trace, err := json.Marshal(responseTrace)
	if err != nil {
		return err
	}
	responseWithTraceExtension, err := jsonparser.Set(data, trace, "extensions", "trace")
	if err != nil {
		return err
	}
	_, err = out.Write(responseWithTraceExtension)
	return err
}

func Do(client *http.Client, ctx context.Context, requestInput []byte, out *bytes.Buffer) (err error) {
	url, method, body, headers, queryParams, enableTrace := requestInputParams(requestInput)
	h := pool.Hash64.Get()
	_, _ = h.Write(body)
	bodyHash := h.Sum64()
	pool.Hash64.Put(h)
	ctx = context.WithValue(ctx, bodyHashContextKey{}, bodyHash)
	return makeHTTPRequest(client, ctx, url, method, headers, queryParams, bytes.NewReader(body), enableTrace, out, ContentTypeJSON)
}

func DoMultipartForm(
	client *http.Client, ctx context.Context, requestInput []byte, files []File, out *bytes.Buffer,
) (err error) {
	if len(files) == 0 {
		return errors.New("no files provided")
	}

	url, method, body, headers, queryParams, enableTrace := requestInputParams(requestInput)

	h := pool.Hash64.Get()
	defer pool.Hash64.Put(h)
	_, _ = h.Write(body)

	formValues := map[string]io.Reader{
		"operations": bytes.NewReader(body),
	}

	var fileMap string
	var tempFiles []*os.File
	for i, file := range files {
		if len(fileMap) == 0 {
			if len(files) == 1 {
				fileMap = fmt.Sprintf(`"%d" : ["variables.file"]`, i)
			} else {
				fileMap = fmt.Sprintf(`"%d" : ["variables.files.%d"]`, i, i)
			}
		} else {
			fileMap = fmt.Sprintf(`%s, "%d" : ["variables.files.%d"]`, fileMap, i, i)
		}
		key := fmt.Sprintf("%d", i)
		_, _ = h.WriteString(file.Path())
		temporaryFile, err := os.Open(file.Path())
		tempFiles = append(tempFiles, temporaryFile)
		if err != nil {
			return err
		}
		formValues[key] = bufio.NewReader(temporaryFile)
	}
	formValues["map"] = strings.NewReader("{ " + fileMap + " }")

	multipartBody, contentType, err := multipartBytes(formValues, files)
	if err != nil {
		return err
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

	bodyHash := h.Sum64()
	ctx = context.WithValue(ctx, bodyHashContextKey{}, bodyHash)

	return makeHTTPRequest(client, ctx, url, method, headers, queryParams, multipartBody, enableTrace, out, contentType)
}

func multipartBytes(values map[string]io.Reader, files []File) (*io.PipeReader, string, error) {
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
