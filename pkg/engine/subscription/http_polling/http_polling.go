package http_polling

import (
	"bytes"
	"context"
	"strconv"
	"time"

	"github.com/buger/jsonparser"
	"github.com/tidwall/sjson"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/httpclient"
)

func SetInputIntervalMillis(input []byte, interval int64) []byte {
	out, _ := sjson.SetRawBytes(input, "interval", []byte(strconv.FormatInt(interval, 10)))
	return out
}

func SetRequestInput(input, requestInput []byte) []byte {
	out, _ := sjson.SetRawBytes(input, "request_input", requestInput)
	return out
}

var (
	HttpPolling = []byte("http_polling_stream")
)

type HttpPollingStream struct {
	client httpclient.Client
}

func New(client httpclient.Client) *HttpPollingStream {
	return &HttpPollingStream{
		client: client,
	}
}

func (h *HttpPollingStream) Start(input []byte, next chan<- []byte, stop <-chan struct{}) {
	interval, err := jsonparser.GetInt(input, "interval")
	if err != nil {
		return
	}
	requestInput, _, _, err := jsonparser.Get(input, "request_input")
	if err != nil {
		return
	}
	buf := bytes.NewBuffer(make([]byte, 0, 4096))
	for {
		select {
		case <-time.After(time.Duration(interval) * time.Millisecond):
			buf.Reset()
			err := h.client.Do(context.Background(), requestInput, buf)
			if err != nil {
				continue
			}
			src := buf.Bytes()
			if len(src) == 0 {
				continue
			}
			dst := make([]byte, len(src))
			copy(dst, src)
			next <- dst
		case <-stop:
			return
		}
	}
}

func (h *HttpPollingStream) UniqueIdentifier() []byte {
	return HttpPolling
}
