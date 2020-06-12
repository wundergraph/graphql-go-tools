package httpclient

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/quotes"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
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

	in = SetInputHeaders(nil, []byte(`{"foo":"bar"}`))
	assert.Equal(t, `{"headers":{"foo":"bar"}}`, string(in))

	in = SetInputHeaders(nil, []byte(`[true]`))
	assert.Equal(t, `{"headers":[true]}`, string(in))

	in = SetInputHeaders(nil, []byte(`[null]`))
	assert.Equal(t, `{"headers":[null]}`, string(in))

	in = SetInputHeaders(nil, []byte(`["str"]`))
	assert.Equal(t, `{"headers":["str"]}`, string(in))

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
}
