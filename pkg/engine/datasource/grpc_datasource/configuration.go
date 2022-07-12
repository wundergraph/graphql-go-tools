package grpc_datasource

import (
	"bytes"
	"encoding/json"
	"net/http"
)

var (
	dot   = []byte(".")
	slash = []byte("/")
)

type Configuration struct {
	Grpc     GrpcConfiguration
	Request  RequestConfiguration
	Protoset []byte
}

type GrpcConfiguration struct {
	Package string
	Service string
	Method  string
	Target  string
}

type RequestConfiguration struct {
	Header http.Header
	Body   string
}

func ConfigJson(config Configuration) json.RawMessage {
	out, _ := json.Marshal(config)
	return out
}

func (c GrpcConfiguration) RpcMethodFullName() string {
	buf := &bytes.Buffer{}
	buf.WriteString(c.Package)
	buf.Write(dot)
	buf.WriteString(c.Service)
	buf.Write(slash)
	buf.WriteString(c.Method)

	return buf.String()
}
