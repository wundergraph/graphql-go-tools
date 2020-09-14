package graphql_websocket_subscription

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"
)

func TestWebsocketClient(t *testing.T) {

	//server := FakeGraphQLSubscriptionServer(t)
	//defer server.Close()

	// host := server.Listener.Addr().String()

	client := &WebsocketClient{}
	err := client.Open("ws", "localhost:4444", "/", nil)
	defer client.Close()
	assert.NoError(t, err)

	totalMessages := &atomic.Int64{}

	subscribe := func(wg *sync.WaitGroup, receiveMessages int) {
		wg.Add(1)
		go func(wg *sync.WaitGroup, receiveMessages int) {
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

		}(wg, receiveMessages)
	}

	wg := &sync.WaitGroup{}

	subscribe(wg, 1)
	subscribe(wg, 2)
	subscribe(wg, 3)

	wg.Wait()

	assert.Equal(t, 0, len(client.subscriptions))
	assert.Equal(t, int64(6), totalMessages.Load())
}

func FakeGraphQLSubscriptionServer(t *testing.T) *httptest.Server {
	upgrader := websocket.Upgrader{}
	subscriptionCounter := atomic.NewInt64(0)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Print("upgrade:", err)
			return
		}
		defer c.Close()
		id := subscriptionCounter.Inc()
		mt, message, err := c.ReadMessage()
		assert.NoError(t, err)
		assert.Equal(t, websocket.TextMessage, mt)
		assert.Equal(t, `{"type":"connection_init"}`, string(message))
		err = c.WriteMessage(websocket.TextMessage, []byte(`{"type":"connection_ack"}`))
		assert.NoError(t, err)
		mt, message, err = c.ReadMessage()
		assert.NoError(t, err)
		assert.Equal(t, websocket.TextMessage, mt)
		assert.Equal(t, fmt.Sprintf(`{"type":"start","id":"%d","payload":{"query":"subscription{counter{count}}"}}`, id), string(message))
		counter := atomic.NewInt64(0)
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			mt, message, err := c.ReadMessage()
			assert.NoError(t, err)
			assert.Equal(t, websocket.TextMessage, mt)
			assert.Equal(t, fmt.Sprintf(`{"id":"%d","type":"stop"}`,id), string(message))
			cancel()
		}()

		for {
			select {
			case <-ctx.Done():
				return
			default:
				err = c.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`{"type":"data","id":"%d","payload":{"data":{"counter":{"count":%d}}}}`, id, counter.Load())))
				assert.NoError(t, err)
				counter.Inc()
			}
		}
	}))
}
