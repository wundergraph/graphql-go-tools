package httpclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/buger/jsonparser"
	bytetemplate "github.com/jensneuse/byte-template"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/wundergraph/graphql-go-tools/pkg/internal/quotes"
	"github.com/wundergraph/graphql-go-tools/pkg/lexer/literal"
)

const (
	PATH                = "path"
	URL                 = "url"
	URLENCODEBODY       = "url_encode_body"
	BASEURL             = "base_url"
	METHOD              = "method"
	BODY                = "body"
	HEADER              = "header"
	QUERYPARAMS         = "query_params"
	USESSE              = "use_sse"
	SSEMETHODPOST       = "sse_method_post"
	SCHEME              = "scheme"
	HOST                = "host"
	UNNULLVARIABLES     = "unnull_variables"
	UNDEFINED_VARIABLES = "undefined"
)

var (
	inputPaths = [][]string{
		{URL},
		{METHOD},
		{BODY},
		{HEADER},
		{QUERYPARAMS},
	}
	subscriptionInputPaths = [][]string{
		{URL},
		{HEADER},
		{BODY},
	}
)

func wrapQuotesIfString(b []byte) []byte {

	if bytes.HasPrefix(b, []byte("$$")) && bytes.HasSuffix(b, []byte("$$")) {
		return b
	}

	if bytes.HasPrefix(b, []byte("{{")) && bytes.HasSuffix(b, []byte("}}")) {
		return b
	}

	inType := gjson.ParseBytes(b).Type
	switch inType {
	case gjson.Number, gjson.String:
		return b
	case gjson.JSON:
		var value interface{}
		withoutTemplate := bytes.ReplaceAll(b, []byte("$$"), nil)

		buf := &bytes.Buffer{}
		tmpl := bytetemplate.New()
		_, _ = tmpl.Execute(buf, withoutTemplate, func(w io.Writer, path []byte) (n int, err error) {
			return w.Write([]byte("0"))
		})

		withoutTemplate = buf.Bytes()

		err := json.Unmarshal(withoutTemplate, &value)
		if err == nil {
			return b
		}
	case gjson.False:
		if bytes.Equal(b, literal.FALSE) {
			return b
		}
	case gjson.True:
		if bytes.Equal(b, literal.TRUE) {
			return b
		}
	case gjson.Null:
		if bytes.Equal(b, literal.NULL) {
			return b
		}
	}
	return quotes.WrapBytes(b)
}

func SetInputURL(input, url []byte) []byte {
	if len(url) == 0 {
		return input
	}
	out, _ := sjson.SetRawBytes(input, URL, wrapQuotesIfString(url))
	return out
}

func SetInputURLEncodeBody(input []byte, urlEncodeBody bool) []byte {
	if !urlEncodeBody {
		return input
	}
	out, _ := sjson.SetRawBytes(input, URLENCODEBODY, []byte("true"))
	return out
}

func SetInputFlag(input []byte, flagName string) []byte {
	out, _ := sjson.SetRawBytes(input, flagName, []byte("true"))
	return out
}

func IsInputFlagSet(input []byte, flagName string) bool {
	value, dataType, _, err := jsonparser.Get(input, flagName)
	if err != nil {
		return false
	}
	if dataType != jsonparser.Boolean {
		return false
	}
	return bytes.Equal(value, literal.TRUE)
}

func SetInputMethod(input, method []byte) []byte {
	if len(method) == 0 {
		return input
	}
	out, _ := sjson.SetRawBytes(input, METHOD, wrapQuotesIfString(method))
	return out
}

func SetInputBody(input, body []byte) []byte {
	return SetInputBodyWithPath(input, body, "")
}

func SetInputBodyWithPath(input, body []byte, path string) []byte {
	if len(body) == 0 {
		return input
	}
	if path != "" {
		path = BODY + "." + path
	} else {
		path = BODY
	}
	out, _ := sjson.SetRawBytes(input, path, wrapQuotesIfString(body))
	return out
}

func SetInputHeader(input, headers []byte) []byte {
	if len(headers) == 0 {
		return input
	}
	out, _ := sjson.SetRawBytes(input, HEADER, wrapQuotesIfString(headers))
	return out
}

func SetInputQueryParams(input, queryParams []byte) []byte {
	if len(queryParams) == 0 {
		return input
	}
	out, _ := sjson.SetRawBytes(input, QUERYPARAMS, wrapQuotesIfString(queryParams))
	return out
}

func SetInputScheme(input, scheme []byte) []byte {
	if len(scheme) == 0 {
		return input
	}
	out, _ := sjson.SetRawBytes(input, SCHEME, wrapQuotesIfString(scheme))
	return out
}

func SetInputHost(input, host []byte) []byte {
	if len(host) == 0 {
		return input
	}
	out, _ := sjson.SetRawBytes(input, HOST, wrapQuotesIfString(host))
	return out
}

func SetInputPath(input, path []byte) []byte {
	if len(path) == 0 {
		return input
	}
	out, _ := sjson.SetRawBytes(input, PATH, wrapQuotesIfString(path))
	return out
}

func requestInputParams(input []byte) (url, method, body, headers, queryParams []byte) {
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
		case 4:
			queryParams = bytes
		}
	}, inputPaths...)
	return
}

func GetSubscriptionInput(input []byte) (url, header, body []byte) {
	jsonparser.EachKey(input, func(i int, bytes []byte, valueType jsonparser.ValueType, err error) {
		switch i {
		case 0:
			url = bytes
		case 1:
			header = bytes
		case 2:
			body = bytes
		}
	}, subscriptionInputPaths...)
	return
}

func setUndefinedVariables(data []byte, undefinedVariables []string) ([]byte, error) {
	if len(undefinedVariables) > 0 {
		encoded, err := json.Marshal(undefinedVariables)
		if err != nil {
			return nil, err
		}
		return sjson.SetRawBytes(data, UNDEFINED_VARIABLES, encoded)
	}
	return data, nil
}

func SetUndefinedVariables(data []byte, undefinedVariables []string) []byte {
	result, err := setUndefinedVariables(data, undefinedVariables)
	if err != nil {
		panic(fmt.Errorf("couldn't set undefined variables: %w", err))
	}
	return result
}

func UndefinedVariables(data []byte) []string {
	var undefinedVariables []string
	gjson.GetBytes(data, UNDEFINED_VARIABLES).ForEach(func(key, value gjson.Result) bool {
		undefinedVariables = append(undefinedVariables, value.Str)
		return true
	})
	return undefinedVariables
}
