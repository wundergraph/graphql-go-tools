package graphql_websocket_subscription

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/buger/jsonparser"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"
)

func TestWebsocketClient(t *testing.T) {
	server := FakeGraphQLSubscriptionServer(t)
	defer server.Close()

	host := server.Listener.Addr().String()

	client := &WebsocketClient{}

	err := client.Open("ws", host, "", nil)
	defer client.Close()
	assert.NoError(t, err)

	totalMessages := &atomic.Int64{}

	subscribe := func(wg *sync.WaitGroup, receiveMessages int) {
		subscription, ok := client.Subscribe([]byte(`{"query":"subscription{counter{count}}"}`))
		assert.True(t, ok)

		template := `{"data":{"counter":{"count":%d}}}`

		for i := 0; i < receiveMessages; i++ {
			expected := fmt.Sprintf(template, i)
			data, ok := subscription.Next(nil)
			assert.True(t, ok)
			assert.Equal(t, expected, string(data))

			totalMessages.Inc()
		}

		client.Unsubscribe(subscription)

		wg.Done()
	}

	wg := &sync.WaitGroup{}
	wg.Add(3)

	go subscribe(wg, 1)
	go subscribe(wg, 2)
	go subscribe(wg, 3)

	wg.Wait()
	assert.Equal(t, 0, len(client.subscriptions))
	assert.Equal(t, int64(6), totalMessages.Load())
}

func FakeGraphQLSubscriptionServer(t *testing.T) *httptest.Server {
	upgrader := websocket.Upgrader{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Print("upgrade:", err)
			return
		}
		defer c.Close()
		mt, message, err := c.ReadMessage()
		assert.NoError(t, err)
		assert.Equal(t, websocket.TextMessage, mt)
		assert.Equal(t, `{"type":"connection_init"}`, string(message))
		err = c.WriteMessage(websocket.TextMessage, []byte(`{"type":"connection_ack"}`))
		assert.NoError(t, err)

		streams := map[string]func(){}
		writeMux := &sync.Mutex{}

		startStream := func(id string, done <-chan struct{}) {
			counter := 0
			for {
				time.Sleep(time.Millisecond)
				select {
				case <-done:
					message := fmt.Sprintf(`{"type":"complete","id":"%s","payload":null}`, id)
					writeMux.Lock()
					err = c.WriteMessage(websocket.TextMessage, []byte(message))
					writeMux.Unlock()
					return
				default:
					message := fmt.Sprintf(`{"type":"data","id":"%s","payload":{"data":{"counter":{"count":%d}}}}`, id, counter)
					writeMux.Lock()
					err = c.WriteMessage(websocket.TextMessage, []byte(message))
					writeMux.Unlock()
					if err != nil {
						return
					}
					counter++
				}
			}
		}

		for {
			mt, message, err := c.ReadMessage()
			if err != nil {
				return
			}
			assert.Equal(t, websocket.TextMessage, mt)

			messageType, err := jsonparser.GetString(message, "type")
			assert.NoError(t, err)

			messageID, err := jsonparser.GetString(message, "id")
			assert.NoError(t, err)

			switch messageType {
			case "start":
				ctx, cancel := context.WithCancel(context.Background())
				streams[messageID] = cancel
				go startStream(messageID, ctx.Done())
			case "stop":
				cancel := streams[messageID]
				cancel()
				delete(streams, messageID)
			}
		}
	}))
}
