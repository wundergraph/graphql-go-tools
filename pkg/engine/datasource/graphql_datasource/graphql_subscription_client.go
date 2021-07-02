package graphql_datasource

import (
	"context"
	"fmt"
	"net/http"

	"github.com/buger/jsonparser"
	"nhooyr.io/websocket"
)

var (
	connectionInitMessage = []byte(`{"type":"connection_init"}`)
)

const (
	startMessage = `{"type":"start","id":"%s","payload":%s}`
	stopMessage  = `{"type":"stop","id":"%s"}`
)

type WebSocketGraphQLSubscriptionClient struct {
	httpClient *http.Client
	ctx        context.Context
}

func NewWebSocketGraphQLSubscriptionClient(httpClient *http.Client, ctx context.Context) *WebSocketGraphQLSubscriptionClient {
	return &WebSocketGraphQLSubscriptionClient{
		httpClient: httpClient,
		ctx:        ctx,
	}
}

func (c *WebSocketGraphQLSubscriptionClient) Subscribe(ctx context.Context, options GraphQLSubscriptionOptions, next chan<- []byte) error {
	isSubscribed := false
	if options.Header == nil {
		options.Header = http.Header{}
	}
	options.Header.Set("Sec-WebSocket-Protocol", "graphql-ws")
	options.Header.Set("Sec-WebSocket-Version", "13")
	conn, response, err := websocket.Dial(ctx, options.URL, &websocket.DialOptions{
		HTTPClient:      c.httpClient,
		HTTPHeader:      options.Header,
		CompressionMode: websocket.CompressionDisabled,
		Subprotocols:    []string{"graphql-ws"},
	})
	if err != nil {
		return err
	}
	defer func() {
		if isSubscribed {
			return
		}
		_ = conn.Write(ctx, websocket.MessageText, []byte(fmt.Sprintf(stopMessage, "1")))
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}()
	if response.StatusCode != http.StatusSwitchingProtocols {
		return fmt.Errorf("upgrade unsuccessful")
	}

	// init + ack
	err = conn.Write(ctx, websocket.MessageText, connectionInitMessage)
	if err != nil {
		return err
	}
	msgType, connectionAckMesage, err := conn.Read(ctx)
	if err != nil {
		return err
	}
	if msgType != websocket.MessageText {
		return fmt.Errorf("unexpected msg type")
	}
	connectionAck, err := jsonparser.GetString(connectionAckMesage, "type")
	if err != nil {
		return err
	}
	if connectionAck != "connection_ack" {
		return fmt.Errorf("expected connection_ack, got: %s", connectionAck)
	}

	// subscribe
	startRequest := fmt.Sprintf(startMessage, "1", options.Body)
	err = conn.Write(ctx, websocket.MessageText, []byte(startRequest))
	if err != nil {
		return err
	}

	isSubscribed = true
	go c.handleSubscription(conn, ctx, next)
	return nil
}

func (c *WebSocketGraphQLSubscriptionClient) handleSubscription(conn *websocket.Conn, ctx context.Context, next chan<- []byte) {
	defer func() {
		_ = conn.Write(ctx, websocket.MessageText, []byte(fmt.Sprintf(stopMessage, "1")))
		_ = conn.Close(websocket.StatusNormalClosure, "")
		close(next)
	}()
	subscriptionLifecycle := ctx.Done()
	resolverLifecycle := c.ctx.Done()
	for {
		select {
		case <-subscriptionLifecycle:
			return
		case <-resolverLifecycle:
			return
		default:
			msgType, data, err := conn.Read(ctx)
			if err != nil {
				continue
			}
			if msgType != websocket.MessageText {
				continue
			}
			messageType, err := jsonparser.GetString(data, "type")
			if err != nil {
				continue
			}
			switch messageType {
			case "data":
				id, err := jsonparser.GetString(data, "id")
				if err != nil {
					continue
				}
				if id != "1" {
					continue
				}
				payload, _, _, err := jsonparser.Get(data, "payload")
				if err != nil {
					continue
				}
				select {
				case next <- payload:
					continue
				case <-subscriptionLifecycle:
					return
				}
			case "complete":
				return
			case "connection_error":
				next <- []byte(`{"errors":[{"message":"connection error"}]}`)
				return
			default:
				continue
			}
		}
	}
}
