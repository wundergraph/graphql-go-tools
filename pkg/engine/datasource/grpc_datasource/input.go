package grpc_datasource

import (
	"bytes"

	"github.com/buger/jsonparser"
	"github.com/tidwall/sjson"

	"github.com/wundergraph/graphql-go-tools/pkg/engine/datasource/httpclient"
)

const (
	PACKAGENAME = "package"
	SERVICE     = "service"
	METHOD      = "method"
	BODY        = "body"
	HEADER      = "header"
)

var (
	dot   = []byte(".")
	slash = []byte("/")
)

var (
	inputPaths = [][]string{
		{PACKAGENAME},
		{SERVICE},
		{METHOD},
		{BODY},
		{HEADER},
	}
)

func RpcCallParams(input []byte) (pkgName, service, method, body, headers []byte) {
	jsonparser.EachKey(input, func(i int, bytes []byte, valueType jsonparser.ValueType, err error) {
		switch i {
		case 0:
			pkgName = bytes
		case 1:
			service = bytes
		case 2:
			method = bytes
		case 3:
			body = bytes
		case 4:
			headers = bytes
		}
	}, inputPaths...)
	return
}

func RpcMethodFullName(pkgName, service, method []byte) string {
	buf := &bytes.Buffer{}
	buf.Write(pkgName)
	buf.Write(dot)
	buf.Write(service)
	buf.Write(slash)
	buf.Write(method)

	return buf.String()
}

func SetInputPackageName(input, pkgName []byte) []byte {
	if len(pkgName) == 0 {
		return input
	}
	out, _ := sjson.SetRawBytes(input, PACKAGENAME, httpclient.WrapQuotesIfString(pkgName))
	return out
}

func SetInputService(input, service []byte) []byte {
	if len(service) == 0 {
		return input
	}
	out, _ := sjson.SetRawBytes(input, SERVICE, httpclient.WrapQuotesIfString(service))
	return out
}

func SetInputMethod(input, method []byte) []byte {
	if len(method) == 0 {
		return input
	}
	out, _ := sjson.SetRawBytes(input, METHOD, httpclient.WrapQuotesIfString(method))
	return out
}

func SetInputBody(input, body []byte) []byte {
	if len(body) == 0 {
		return input
	}
	out, _ := sjson.SetRawBytes(input, BODY, httpclient.WrapQuotesIfString(body))
	return out
}

func SetInputHeader(input, headers []byte) []byte {
	if len(headers) == 0 {
		return input
	}
	out, _ := sjson.SetRawBytes(input, HEADER, httpclient.WrapQuotesIfString(headers))
	return out
}
