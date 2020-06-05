package datasource

import (
	"context"
	"io"

	"github.com/tidwall/sjson"
)

const (
	PATH        = "path"
	URL         = "url"
	BASEURL     = "base_url"
	METHOD      = "method"
	BODY        = "body"
	HEADERS     = "headers"
	QUERYPARAMS = "query_params"
)

type Client interface {
	Do(ctx context.Context, requestInput []byte, out io.Writer) (err error)
}

func SetInputPath(input, path []byte) []byte {
	out, _ := sjson.SetRawBytes(input, PATH, path)
	return out
}

func SetInputURL(input,url []byte) []byte {
	out, _ := sjson.SetRawBytes(input, URL, url)
	return out
}

func SetInputMethod(input,method []byte) []byte {
	out, _ := sjson.SetRawBytes(input, METHOD, method)
	return out
}

func SetInputBody(input,body []byte) []byte {
	out, _ := sjson.SetRawBytes(input, URL, body)
	return out
}

func SetInputHeaders(input,headers []byte) []byte {
	out, _ := sjson.SetRawBytes(input, HEADERS, headers)
	return out
}