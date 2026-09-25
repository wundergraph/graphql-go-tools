package httpclient

import (
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tidwall/sjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/quotes"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
)

func TestHttpClient(t *testing.T) {
	in := SetInputMethod(nil, literal.HTTP_METHOD_GET)
	assert.Equal(t, `{"method":"GET"}`, string(in))

	in = SetInputMethod(nil, quotes.WrapBytes(literal.HTTP_METHOD_POST))
	assert.Equal(t, `{"method":"POST"}`, string(in))

	in = SetInputURL(nil, []byte("foo.bar.com"))
	assert.Equal(t, `{"url":"foo.bar.com"}`, string(in))

	in = SetInputURL(nil, []byte("\"foo.bar.com\""))
	assert.Equal(t, `{"url":"foo.bar.com"}`, string(in))

	in = SetInputQueryParams(nil, []byte(`{"foo":"bar"}`))
	assert.Equal(t, `{"query_params":{"foo":"bar"}}`, string(in))

	in = SetInputHeader(nil, []byte(`{"foo":"bar"}`))
	assert.Equal(t, `{"header":{"foo":"bar"}}`, string(in))

	in = SetInputHeader(nil, []byte(`[true]`))
	assert.Equal(t, `{"header":[true]}`, string(in))

	in = SetInputHeader(nil, []byte(`[null]`))
	assert.Equal(t, `{"header":[null]}`, string(in))

	in = SetInputHeader(nil, []byte(`["str"]`))
	assert.Equal(t, `{"header":["str"]}`, string(in))

	in = SetInputBody(nil, []byte(`{"foo":"bar"}`))
	assert.Equal(t, `{"body":{"foo":"bar"}}`, string(in))

	in = SetInputBodyWithPath(nil, []byte(`{"foo":"bar"}`), "variables")
	assert.Equal(t, `{"body":{"variables":{"foo":"bar"}}}`, string(in))

	in = SetInputBodyWithPath(nil, []byte(`query { foo }`), "query")
	assert.Equal(t, `{"body":{"query":"query { foo }"}}`, string(in))

	in = SetInputBodyWithPath(nil, []byte(`{ foo }`), "query")
	assert.Equal(t, `{"body":{"query":"{ foo }"}}`, string(in))

	in = SetInputBodyWithPath(nil, []byte(`{foo}`), "query")
	assert.Equal(t, `{"body":{"query":"{foo}"}}`, string(in))

	in = SetInputBodyWithPath(nil, []byte(`{`), "query")
	assert.Equal(t, `{"body":{"query":"{"}}`, string(in))

	in = SetInputBodyWithPath(nil, []byte(`{topProducts {upc name price}}}`), "query")
	assert.Equal(t, `{"body":{"query":"{topProducts {upc name price}}}"}}`, string(in))

	in = SetInputBodyWithPath(nil, []byte(`$$0$$`), "variables.foo")
	assert.Equal(t, `{"body":{"variables":{"foo":$$0$$}}}`, string(in))

	in = SetInputBodyWithPath(nil, []byte(`"$$0$$"`), "variables.foo")
	assert.Equal(t, `{"body":{"variables":{"foo":"$$0$$"}}}`, string(in))

	in = SetInputBodyWithPath(nil, []byte(`{"bar":$$0$$}`), "variables.foo")
	assert.Equal(t, `{"body":{"variables":{"foo":{"bar":$$0$$}}}}`, string(in))
}

func TestHttpClientDo(t *testing.T) {

	runTest := func(ctx context.Context, input []byte, expectedOutput string) func(t *testing.T) {
		return func(t *testing.T) {
			output, err := Do(http.DefaultClient, ctx, nil, input)
			assert.NoError(t, err)
			assert.Equal(t, expectedOutput, string(output))
		}
	}

	background := context.Background()

	t.Run("simple get", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := httputil.DumpRequest(r, true)
			assert.NoError(t, err)
			_, err = w.Write([]byte("ok"))
			assert.NoError(t, err)
		}))
		defer server.Close()
		var input []byte
		input = SetInputMethod(input, []byte("GET"))
		input = SetInputURL(input, []byte(server.URL))
		t.Run("net", runTest(background, input, `ok`))
	})

	t.Run("query params simple", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fooValues := r.URL.Query()["foo"]
			assert.Len(t, fooValues, 1)
			assert.Equal(t, fooValues[0], "bar")
			_, err := w.Write([]byte("ok"))
			assert.NoError(t, err)
		}))
		defer server.Close()
		var input []byte
		input = SetInputMethod(input, []byte("GET"))
		input = SetInputURL(input, []byte(server.URL))
		input = SetInputQueryParams(input, []byte(`[{"name":"foo","value":"bar"}]`))
		t.Run("net", runTest(background, input, `ok`))
	})

	t.Run("query params multiple", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fooValues := r.URL.Query()["foo"]
			assert.Len(t, fooValues, 2)
			assert.Equal(t, fooValues[0], "bar")
			assert.Equal(t, fooValues[1], "baz")

			yearValues := r.URL.Query()["year"]
			assert.Len(t, yearValues, 1)
			assert.Equal(t, yearValues[0], "2020")

			_, err := w.Write([]byte("ok"))
			assert.NoError(t, err)
		}))
		defer server.Close()
		var input []byte
		input = SetInputMethod(input, []byte("GET"))
		input = SetInputURL(input, []byte(server.URL))
		input = SetInputQueryParams(input, []byte(`[{"name":"foo","value":"bar"},{"name":"foo","value":"baz"},{"name":"year","value":"2020"}]`))
		t.Run("net", runTest(background, input, `ok`))
	})

	t.Run("query params multiple as array", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fooValues := r.URL.Query()["foo"]
			assert.Len(t, fooValues, 2)
			assert.Equal(t, fooValues[0], "bar")
			assert.Equal(t, fooValues[1], "baz")
			_, err := w.Write([]byte("ok"))
			assert.NoError(t, err)
		}))
		defer server.Close()
		var input []byte
		input = SetInputMethod(input, []byte("GET"))
		input = SetInputURL(input, []byte(server.URL))
		input = SetInputQueryParams(input, []byte(`[{"name":"foo","value":["bar","baz"]}]`))
		t.Run("net", runTest(background, input, `ok`))
	})

	t.Run("post", func(t *testing.T) {
		body := []byte(`{"foo":"bar"}`)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := w.Write([]byte("ok"))
			assert.NoError(t, err)
			actualBody, err := io.ReadAll(r.Body)
			assert.NoError(t, err)
			assert.Equal(t, string(body), string(actualBody))
		}))
		defer server.Close()
		var input []byte
		input = SetInputMethod(input, []byte("POST"))
		input = SetInputBody(input, body)
		input = SetInputURL(input, []byte(server.URL))
		t.Run("net", runTest(background, input, `ok`))
	})

	t.Run("gzip", func(t *testing.T) {
		body := []byte(`{"foo":"bar"}`)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			acceptEncoding := r.Header.Get("Accept-Encoding")
			assert.Equal(t, "gzip", acceptEncoding)
			actualBody, err := io.ReadAll(r.Body)
			assert.NoError(t, err)
			assert.Equal(t, string(body), string(actualBody))
			gzipWriter := gzip.NewWriter(w)
			defer gzipWriter.Close()
			w.Header().Set("Content-Encoding", "gzip")
			_, err = gzipWriter.Write([]byte("ok"))
			assert.NoError(t, err)
		}))
		defer server.Close()
		var input []byte
		input = SetInputMethod(input, []byte("POST"))
		input = SetInputBody(input, body)
		input = SetInputURL(input, []byte(server.URL))
		t.Run("net", runTest(background, input, `ok`))
	})

	t.Run("redact sensitive headers", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := httputil.DumpRequest(r, true)
			assert.NoError(t, err)
			w.Header().Set("Authorization", "test")
			_, err = w.Write([]byte(`{"extensions": {"trace": {}}"}`))
			assert.NoError(t, err)
		}))
		defer server.Close()
		var input []byte
		input = SetInputMethod(input, []byte("GET"))
		input = SetInputURL(input, []byte(server.URL))
		input, err := sjson.SetBytes(input, TRACE, true)
		assert.NoError(t, err)
		output, err := Do(http.DefaultClient, context.Background(), nil, input)
		assert.NoError(t, err)
		assert.Contains(t, string(output), `"Authorization":["****"]`)
	})
}

func TestAssembleGraphQLRequestInput(t *testing.T) {
	variables := []byte(`{"representations":[$$0$$]}`)
	query := []byte(`query{me{id}}`)

	t.Run("with header", func(t *testing.T) {
		input := AssembleGraphQLRequestInput(variables, query, []byte(`{"Authorization":["secret"]}`), "https://example.com/graphql", "POST")
		assert.Equal(t, `{"method":"POST","url":"https://example.com/graphql","header":{"Authorization":["secret"]},"body":{"query":"query{me{id}}","variables":{"representations":[$$0$$]}}}`, string(input))
	})

	t.Run("without header", func(t *testing.T) {
		input := AssembleGraphQLRequestInput(variables, query, nil, "https://example.com/graphql", "POST")
		assert.Equal(t, `{"method":"POST","url":"https://example.com/graphql","body":{"query":"query{me{id}}","variables":{"representations":[$$0$$]}}}`, string(input))
	})

	t.Run("null header omitted like without header", func(t *testing.T) {
		input := AssembleGraphQLRequestInput(variables, query, []byte("null"), "https://example.com/graphql", "POST")
		assert.Equal(t, `{"method":"POST","url":"https://example.com/graphql","body":{"query":"query{me{id}}","variables":{"representations":[$$0$$]}}}`, string(input))
	})
}

func TestAssembleGraphQLRequestInputSplitVariables(t *testing.T) {
	query := []byte(`query{me{id}}`)

	t.Run("with header", func(t *testing.T) {
		prefix, suffix, err := AssembleGraphQLRequestInputSplitVariables(query, []byte(`{"Authorization":["secret"]}`), "https://example.com/graphql", "POST")
		assert.NoError(t, err)
		assert.Equal(t, `{"method":"POST","url":"https://example.com/graphql","header":{"Authorization":["secret"]},"body":{"query":"query{me{id}}","variables":{`, prefix)
		assert.Equal(t, `}}}`, suffix)
	})

	t.Run("without header", func(t *testing.T) {
		prefix, suffix, err := AssembleGraphQLRequestInputSplitVariables(query, nil, "https://example.com/graphql", "POST")
		assert.NoError(t, err)
		assert.Equal(t, `{"method":"POST","url":"https://example.com/graphql","body":{"query":"query{me{id}}","variables":{`, prefix)
		assert.Equal(t, `}}}`, suffix)
	})

	// A header whose raw JSON contains the literal text `"variables":{}` would
	// defeat a first-occurrence marker search: sjson prepends, so the header sits
	// BEFORE the body slot in the assembled document. The tail-based split lands
	// on the body slot regardless.
	t.Run("header value containing the variables marker", func(t *testing.T) {
		header := []byte(`{"X-Decoy":{"variables":{}}}`)
		prefix, suffix, err := AssembleGraphQLRequestInputSplitVariables(query, header, "https://example.com/graphql", "POST")
		assert.NoError(t, err)
		assert.Equal(t, `{"method":"POST","url":"https://example.com/graphql","header":{"X-Decoy":{"variables":{}}},"body":{"query":"query{me{id}}","variables":{`, prefix)
		assert.Equal(t, `}}}`, suffix)
	})

	// Reassembling prefix + rendered variables + suffix must be byte-identical to
	// a plain AssembleGraphQLRequestInput call with the same variables.
	t.Run("prefix and suffix reassemble to the unsplit envelope", func(t *testing.T) {
		header := []byte(`{"Authorization":["secret"]}`)
		prefix, suffix, err := AssembleGraphQLRequestInputSplitVariables(query, header, "https://example.com/graphql", "POST")
		assert.NoError(t, err)
		unsplit := AssembleGraphQLRequestInput([]byte(`{"representations":[$$0$$]}`), query, header, "https://example.com/graphql", "POST")
		assert.Equal(t, string(unsplit), prefix+`"representations":[$$0$$]`+suffix)
	})
}
