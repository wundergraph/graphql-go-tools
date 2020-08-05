package http_polling

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/httpclient"
)

func TestHttpPolling(t *testing.T) {
	counter := 0
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strconv.Itoa(counter)))
		counter++
	}))
	defer testServer.Close()
	manager := NewManager(httpclient.NewNetHttpClient(httpclient.DefaultNetHttpClient))
	var (
		requestInput []byte
		input        []byte
		out          bytes.Buffer
	)
	requestInput = httpclient.SetInputURL(requestInput, []byte(testServer.URL))
	requestInput = httpclient.SetInputMethod(requestInput, []byte("GET"))

	input = SetInputInterval(input, 0)
	input = SetRequestInput(input, requestInput)

	trigger, err := manager.ConfigureTrigger(input)
	assert.NoError(t, err)

	err = trigger.Next(context.Background(), &out)
	assert.NoError(t, err)
	assert.Equal(t, "0", out.String())
	out.Reset()

	err = trigger.Next(context.Background(), &out)
	assert.NoError(t, err)
	assert.Equal(t, "1", out.String())
	out.Reset()

	err = trigger.Next(context.Background(), &out)
	assert.NoError(t, err)
	assert.Equal(t, "2", out.String())
	out.Reset()

	assert.Equal(t,3,counter)
}
