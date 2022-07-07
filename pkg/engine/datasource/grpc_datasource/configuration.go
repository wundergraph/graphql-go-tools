package grpc_datasource

import (
	"bytes"
	"encoding/json"
)

var (
	dot   = []byte(".")
	slash = []byte("/")
)

type Configuration struct {
	Package  string
	Service  string
	Method   string
	Target   string
	Protoset []byte
}

func ConfigJson(config Configuration) json.RawMessage {
	out, _ := json.Marshal(config)
	return out
}

func (c Configuration) RpcMethodFullName() string {
	buf := &bytes.Buffer{}
	buf.WriteString(c.Package)
	buf.Write(dot)
	buf.WriteString(c.Service)
	buf.Write(slash)
	buf.WriteString(c.Target)

	return buf.String()
}
