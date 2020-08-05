package http_polling

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/buger/jsonparser"
	"github.com/tidwall/sjson"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/httpclient"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
)

func SetInputInterval(input []byte, interval int64) []byte {
	out, _ := sjson.SetRawBytes(input, "interval", []byte(strconv.FormatInt(interval, 10)))
	return out
}

func SetRequestInput(input, requestInput []byte) []byte {
	out, _ := sjson.SetRawBytes(input, "request_input", requestInput)
	return out
}

type Trigger struct {
	client       httpclient.Client
	requestInput []byte
	interval     time.Duration
}

func (h *Trigger) Next(ctx context.Context, out io.Writer) (err error) {
	done := ctx.Done()
	for {
		select {
		case <-done:
			return nil
		case <-time.After(h.interval):
			return h.client.Do(ctx, h.requestInput, out)
		}
	}
}

type Manager struct {
	client httpclient.Client
}

var (
	UniqueID = []byte("http_polling")
)

func (m *Manager) UniqueIdentifier() []byte {
	return UniqueID
}

func (m *Manager) ConfigureTrigger(input []byte) (trigger resolve.SubscriptionTrigger, err error) {
	interval, err := jsonparser.GetInt(input, "interval")
	if err != nil {
		return nil, fmt.Errorf("interval missing")
	}
	requestInput, _, _, err := jsonparser.Get(input, "request_input")
	if err != nil {
		return nil, fmt.Errorf("request_input missing")
	}
	return &Trigger{
		requestInput: requestInput,
		interval:     time.Millisecond * time.Duration(interval),
		client:       m.client,
	}, nil
}

func NewManager(client httpclient.Client) *Manager {
	return &Manager{
		client: client,
	}
}
