package httpclient

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/quotes"
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
			out := &bytes.Buffer{}
			err := Do(http.DefaultClient, ctx, input, out)
			assert.NoError(t, err)
			assert.Equal(t, expectedOutput, out.String())
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
}
