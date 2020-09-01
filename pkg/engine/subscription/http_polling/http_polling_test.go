package http_polling

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/httpclient"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/subscription"
)

func TestHttpPolling(t *testing.T) {
	counter := 0
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strconv.Itoa(counter)))
		counter++
	}))
	defer testServer.Close()

	httpPollingStream := New(httpclient.NewNetHttpClient(httpclient.DefaultNetHttpClient))

	manager := subscription.NewManager(httpPollingStream)
	var (
		requestInput []byte
		input        []byte
	)
	requestInput = httpclient.SetInputURL(requestInput, []byte(testServer.URL))
	requestInput = httpclient.SetInputMethod(requestInput, []byte("GET"))

	input = SetInputIntervalMillis(input, 0)
	input = SetRequestInput(input, requestInput)

	trigger1, err := manager.StartTrigger(input)
	assert.NoError(t, err)

	trigger2, err := manager.StartTrigger(input)
	assert.NoError(t, err)

	trigger3, err := manager.StartTrigger(input)
	assert.NoError(t, err)

	receiveOneAndStop := func(trigger *subscription.Trigger, wg *sync.WaitGroup, triggerID int) {
		data, ok := trigger.Next(context.Background())
		assert.True(t, ok)
		assert.Equal(t, "0", string(data), "triggerID: %d", triggerID)

		manager.StopTrigger(trigger)

		wg.Done()
	}

	receive := func(trigger *subscription.Trigger, wg *sync.WaitGroup, triggerID int) {
		data, ok := trigger.Next(context.Background())
		assert.True(t, ok)
		assert.Equal(t, "0", string(data), "triggerID: %d", triggerID)

		data, ok = trigger.Next(context.Background())
		assert.True(t, ok)
		assert.Equal(t, "1", string(data), "triggerID: %d", triggerID)

		data, ok = trigger.Next(context.Background())
		assert.True(t, ok)
		assert.Equal(t, "2", string(data), "triggerID: %d", triggerID)

		wg.Done()
	}

	wg := &sync.WaitGroup{}
	wg.Add(3)

	go receive(trigger1, wg, 1)
	go receive(trigger2, wg, 2)
	go receiveOneAndStop(trigger3, wg, 3)

	wg.Wait()

	assert.Equal(t, 3, counter)

	trigger4, err := manager.StartTrigger(input)
	assert.NoError(t, err)

	manager.StopTrigger(trigger1)
	manager.StopTrigger(trigger2)

	data, ok := trigger4.Next(context.Background())
	assert.True(t, ok)
	assert.Equal(t, "3", string(data), "triggerID: %d", 4)

	manager.StopTrigger(trigger4)
}
