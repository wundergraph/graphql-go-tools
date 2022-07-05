package grpc_datasource

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRpcCallParams(t *testing.T) {
	in := []byte(`{"package":"lucasfilms","service":"StartwarsService","method":"GetHero","body":{"foo":"bar"},"header":{"fizz":"buzz"}}`)

	pkgName, service, method, body, headers := RpcCallParams(in)

	assert.Equal(t, []byte("lucasfilms"), pkgName)
	assert.Equal(t, []byte("StartwarsService"), service)
	assert.Equal(t, []byte("GetHero"), method)
	assert.Equal(t, []byte(`{"foo":"bar"}`), body)
	assert.Equal(t, []byte(`{"fizz":"buzz"}`), headers)
}

func TestFullRpcMethodName(t *testing.T) {
	assert.Equal(t, "lucasfilms.StartwarsService/GetHero", RpcMethodFullName([]byte("lucasfilms"), []byte("StartwarsService"), []byte("GetHero")))
}

func TestRpcInput(t *testing.T) {
	in := SetInputPackageName(nil, []byte("lucasfilms"))
	assert.Equal(t, `{"package":"lucasfilms"}`, string(in))

	in = SetInputService(nil, []byte("StartwarsService"))
	assert.Equal(t, `{"service":"StartwarsService"}`, string(in))

	in = SetInputMethod(nil, []byte("GetHero"))
	assert.Equal(t, `{"method":"GetHero"}`, string(in))

	in = SetInputBody(nil, []byte(`{"foo":"bar"}`))
	assert.Equal(t, `{"body":{"foo":"bar"}}`, string(in))

	in = SetInputHeader(nil, []byte(`{"foo":"bar"}`))
	assert.Equal(t, `{"header":{"foo":"bar"}}`, string(in))

	in = SetInputHeader(nil, []byte(`[true]`))
	assert.Equal(t, `{"header":[true]}`, string(in))

	in = SetInputHeader(nil, []byte(`[null]`))
	assert.Equal(t, `{"header":[null]}`, string(in))

	in = SetInputHeader(nil, []byte(`["str"]`))
	assert.Equal(t, `{"header":["str"]}`, string(in))
}
