package graphql_websocket_subscription

import (
	"fmt"
	"hash"
	"math"
	"net/http"
	"net/url"

	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash"
	"github.com/gorilla/websocket"
)

type GraphQLWebsocketSubscriptionStream struct {
	register       chan registerSubscription
	unregister     chan subscriptionInfo
	hash           hash.Hash64
	connections    map[uint64]*connection
	clientsPerConn map[uint64]uint64
}

func New() *GraphQLWebsocketSubscriptionStream {
	return &GraphQLWebsocketSubscriptionStream{
		register:       make(chan registerSubscription),
		unregister:     make(chan subscriptionInfo),
		hash:           xxhash.New(),
		connections:    map[uint64]*connection{},
		clientsPerConn: map[uint64]uint64{},
	}
}

var (
	uniqueIdentifier      = []byte("graphql_websocket_subscription")
	connectionInitMessage = []byte(`{"type":"connection_init"}`)
	startMessage          = []byte(`{"type":"start","id":1,"payload":{"query":"subscription{counter{count}}"}}`)
	stopMessage           = []byte(`{"type":"stop","connectionID":1}`)
)

type startSubscription struct {
	connectionID      uint64
	input             []byte
	next              chan<- []byte
	getSubscriptionID chan uint64
}

type connection struct {
	conn              *websocket.Conn
	startSubscription chan startSubscription
	stopSubscription  chan subscriptionInfo
	subscriptions     map[uint64]chan<- []byte
}

func (c *connection) Run(done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		case start := <-c.startSubscription:
			id, err := c.nextSubscriptionID()
			if err != nil {
				close(start.getSubscriptionID)
				if len(c.subscriptions) == 0 {
					return
				}
				continue
			}
			c.subscriptions[id] = start.next
			// TODO: use actual payload
			err = c.conn.WriteMessage(websocket.TextMessage, startMessage)
			if err != nil {
				close(start.getSubscriptionID)
				if len(c.subscriptions) == 0 {
					return
				}
				continue
			}
			start.getSubscriptionID <- id
		case stop := <-c.stopSubscription:
			err := c.conn.WriteMessage(websocket.TextMessage, stopMessage)
			if err != nil {
				continue
			}
			delete(c.subscriptions, stop.subscriptionID)
			if len(c.subscriptions) == 0 {
				return
			}
		default:
			c.readDispatchNextMessage()
		}
	}
}

func (c *connection) nextSubscriptionID() (uint64, error) {
	for i := uint64(1); i < math.MaxInt64; i++ {
		_, exists := c.subscriptions[i]
		if exists {
			continue
		}
		return i, nil
	}
	return 0, fmt.Errorf("too many subscriptions")
}

func (c *connection) readDispatchNextMessage() {
	_, data, err := c.conn.ReadMessage()
	if err != nil {
		return
	}
	dataType, err := jsonparser.GetString(data, "type")
	if err != nil {
		return
	}
	if dataType != "data" {
		return
	}
	payload, _, _, err := jsonparser.Get(data, "payload")
	if err != nil {
		return
	}
	id, err := jsonparser.GetInt(data, "id")
	if err != nil {
		return
	}
	sub, ok := c.subscriptions[uint64(id)]
	if !ok {
		return
	}
	sub <- payload
}

type registerSubscription struct {
	info  chan subscriptionInfo
	input []byte
	next  chan<- []byte
}

type subscriptionInfo struct {
	connectionID   uint64
	subscriptionID uint64
}

func (g *GraphQLWebsocketSubscriptionStream) Start(input []byte, next chan<- []byte, stop <-chan struct{}) {

	getInfo := make(chan subscriptionInfo)

	g.register <- registerSubscription{
		input: input,
		next:  next,
		info:  getInfo,
	} // register the subscription with the upstream

	info := <-getInfo // get the sub ID

	<-stop // wait until all clients disconnected

	g.unregister <- info // unregister the subscription from the upstream
}

func (g *GraphQLWebsocketSubscriptionStream) Run(done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		case registration := <-g.register:
			config := g.getWebsocketConfig(registration.input)
			id := g.connectionID(config)
			getSubscriptionID := make(chan uint64)
			conn, ok := g.connections[id]
			if ok {
				conn.startSubscription <- startSubscription{
					connectionID:      id,
					input:             registration.input,
					next:              registration.next,
					getSubscriptionID: getSubscriptionID,
				}
				subscriptionID, ok := <-getSubscriptionID
				if !ok {
					continue
				}
				g.clientsPerConn[id] += 1
				registration.info <- subscriptionInfo{
					connectionID:   id,
					subscriptionID: subscriptionID,
				}
				continue
			}
			wsConn, err := g.startConnection(config)
			if err != nil {
				continue
			}
			conn = &connection{
				conn:              wsConn,
				startSubscription: make(chan startSubscription),
				stopSubscription:  make(chan subscriptionInfo),
				subscriptions:     map[uint64]chan<- []byte{},
			}
			go conn.Run(done)
			conn.startSubscription <- startSubscription{
				connectionID:      id,
				input:             registration.input,
				next:              registration.next,
				getSubscriptionID: getSubscriptionID,
			}
			subscriptionID, ok := <-getSubscriptionID
			if !ok {
				continue
			}
			g.connections[id] = conn
			g.clientsPerConn[id] = 1
			registration.info <- subscriptionInfo{
				connectionID:   id,
				subscriptionID: subscriptionID,
			}
		case unregister := <-g.unregister:
			g.connections[unregister.connectionID].stopSubscription <- unregister
			clients := g.clientsPerConn[unregister.connectionID] - 1
			if clients == 0 {
				return
			}
			g.clientsPerConn[unregister.connectionID] = clients
		}
	}
}

func (g *GraphQLWebsocketSubscriptionStream) startConnection(config websocketConfig) (*websocket.Conn, error) {
	u := url.URL{
		Scheme: string(config.scheme),
		Host:   string(config.host),
		Path:   string(config.path),
	}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), http.Header{
		"Sec-WebSocket-Protocol": []string{"graphql-ws"},
		"Sec-WebSocket-Version":  []string{"13"},
	})
	return c, err
}

func (g *GraphQLWebsocketSubscriptionStream) connectionID(config websocketConfig) uint64 {
	g.hash.Reset()
	_, _ = g.hash.Write(config.scheme)
	_, _ = g.hash.Write(config.host)
	_, _ = g.hash.Write(config.path)
	return g.hash.Sum64()
}

var (
	configKeys = [][]string{
		{"scheme"},
		{"host"},
		{"path"},
	}
)

type websocketConfig struct {
	scheme, host, path []byte
}

func (g *GraphQLWebsocketSubscriptionStream) getWebsocketConfig(input []byte) (cfg websocketConfig) {
	jsonparser.EachKey(input, func(i int, bytes []byte, valueType jsonparser.ValueType, err error) {
		switch i {
		case 0:
			cfg.scheme = bytes
		case 1:
			cfg.host = bytes
		case 2:
			cfg.path = bytes
		}
	}, configKeys...)
	return
}

func (g *GraphQLWebsocketSubscriptionStream) closeCon(c *websocket.Conn) {
	_ = c.WriteMessage(websocket.TextMessage, stopMessage)
	_ = c.Close()
}

func (g *GraphQLWebsocketSubscriptionStream) init(c *websocket.Conn) error {
	err := c.WriteMessage(websocket.TextMessage, connectionInitMessage)
	if err != nil {
		return err
	}

	_, connectionAckMessage, err := c.ReadMessage()
	if err != nil {
		return err
	}

	connectionAck, err := jsonparser.GetString(connectionAckMessage, "type")
	if err != nil {
		return err
	}

	if connectionAck != "connection_ack" {
		return fmt.Errorf("ws connection_init not acked")
	}

	return nil
}

func (g *GraphQLWebsocketSubscriptionStream) UniqueIdentifier() []byte {
	return uniqueIdentifier
}
